package hostbackend

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/mirusu400/aram-frontend/frontend"
)

// recordingAdapter stands in for one half of the product. It implements the
// whole optional contract so a test can tell which adapter a call reached
// rather than whether an interface assertion happened to succeed.
type recordingAdapter struct {
	name     string
	opens    []frontend.OpenRequest
	closes   int
	audio    []frontend.AudioSettings
	fonts    []frontend.FontSettings
	cpus     []frontend.CPUSettings
	frames   int
	openErr  error
	closeErr error
	icons    []string
	requests []frontend.ExternalRequest
	cleared  []uint64
}

// Icon records the requested path and returns bytes tagged with this adapter's
// name so a test can prove which half answered.
func (a *recordingAdapter) Icon(path string) ([]byte, error) {
	a.icons = append(a.icons, path)
	return []byte("icon:" + a.name), nil
}

func (a *recordingAdapter) Open(
	_ context.Context,
	request frontend.OpenRequest,
) (frontend.InputInfo, error) {
	a.opens = append(a.opens, request)
	return frontend.InputInfo{DisplayName: a.name}, a.openErr
}

func (a *recordingAdapter) State() frontend.BackendState                           { return frontend.StateReady }
func (a *recordingAdapter) Supports(frontend.BackendCommand) bool                  { return true }
func (a *recordingAdapter) Execute(context.Context, frontend.BackendCommand) error { return nil }

func (a *recordingAdapter) Close() error {
	a.closes++
	return a.closeErr
}

func (a *recordingAdapter) ConfigureAudio(settings frontend.AudioSettings) error {
	a.audio = append(a.audio, settings)
	return nil
}

func (a *recordingAdapter) ConfigureFont(settings frontend.FontSettings) error {
	a.fonts = append(a.fonts, settings)
	return nil
}

func (a *recordingAdapter) ConfigureCPU(settings frontend.CPUSettings) error {
	a.cpus = append(a.cpus, settings)
	return nil
}

func (a *recordingAdapter) AvailableCPUBackends() []string { return []string{a.name} }

func (a *recordingAdapter) RunFrame(context.Context) error {
	a.frames++
	return nil
}

func (a *recordingAdapter) ExternalRequests() []frontend.ExternalRequest {
	return append([]frontend.ExternalRequest(nil), a.requests...)
}

func (a *recordingAdapter) ClearExternalRequests(through uint64) {
	a.cleared = append(a.cleared, through)
	for len(a.requests) != 0 && a.requests[0].Sequence <= through {
		a.requests = a.requests[1:]
	}
}

func (a *recordingAdapter) BackendName() string { return a.name }

// plainAdapter implements only the required contract, so the forwarding paths
// that depend on an optional interface can be checked for their fallbacks.
type plainAdapter struct{ recordingAdapter }

func (a *plainAdapter) ConfigureAudio(frontend.AudioSettings) error { panic("not implemented") }
func (a *plainAdapter) ConfigureFont(frontend.FontSettings) error   { panic("not implemented") }
func (a *plainAdapter) ConfigureCPU(frontend.CPUSettings) error     { panic("not implemented") }
func (a *plainAdapter) AvailableCPUBackends() []string              { panic("not implemented") }
func (a *plainAdapter) RunFrame(context.Context) error              { panic("not implemented") }

type blockingOpenAdapter struct {
	*recordingAdapter
	mu          sync.Mutex
	inFlight    int
	maxInFlight int
	started     chan struct{}
	release     chan struct{}
}

type blockingSnapshotAdapter struct {
	*recordingAdapter
	once    sync.Once
	started chan struct{}
	release chan struct{}
}

func (a *blockingSnapshotAdapter) ExternalRequests() []frontend.ExternalRequest {
	a.once.Do(func() {
		close(a.started)
		<-a.release
	})
	return a.recordingAdapter.ExternalRequests()
}

type blockingClearAdapter struct {
	*recordingAdapter
	once    sync.Once
	started chan struct{}
	release chan struct{}
}

func (a *blockingClearAdapter) ClearExternalRequests(through uint64) {
	a.once.Do(func() {
		close(a.started)
		<-a.release
	})
	a.recordingAdapter.ClearExternalRequests(through)
}

func (a *blockingOpenAdapter) Open(
	_ context.Context,
	request frontend.OpenRequest,
) (frontend.InputInfo, error) {
	a.mu.Lock()
	a.inFlight++
	if a.inFlight > a.maxInFlight {
		a.maxInFlight = a.inFlight
	}
	a.recordingAdapter.opens = append(a.recordingAdapter.opens, request)
	a.mu.Unlock()
	a.started <- struct{}{}
	<-a.release
	a.mu.Lock()
	a.inFlight--
	a.mu.Unlock()
	return frontend.InputInfo{DisplayName: a.name}, nil
}

func newTestBackend() (*Backend, *recordingAdapter, *recordingAdapter) {
	application := &recordingAdapter{name: "application"}
	system := &recordingAdapter{name: "system"}
	return newBackend(application, system), application, system
}

func TestOpenRoutesByRequestKind(t *testing.T) {
	backend, application, system := newTestBackend()
	ctx := context.Background()

	if _, err := backend.Open(ctx, frontend.OpenRequest{Path: "title.zip"}); err != nil {
		t.Fatal(err)
	}
	if len(application.opens) != 1 || len(system.opens) != 0 {
		t.Fatalf("title open went to application=%d system=%d", len(application.opens), len(system.opens))
	}

	if _, err := backend.Open(ctx, frontend.OpenRequest{Path: "fw", Firmware: true}); err != nil {
		t.Fatal(err)
	}
	if len(system.opens) != 1 {
		t.Fatalf("firmware open reached the system adapter %d times", len(system.opens))
	}
	if backend.BackendName() != "system" {
		t.Fatalf("active adapter after a firmware open = %q", backend.BackendName())
	}
}

func TestExternalRequestsRouteToActiveAdapter(t *testing.T) {
	backend, application, system := newTestBackend()
	application.requests = []frontend.ExternalRequest{{
		Sequence: 8, Kind: frontend.ExternalRequestBrowser, Data: []byte{1, 2, 3},
	}}
	requests := backend.ExternalRequests()
	if len(requests) != 1 || requests[0].Sequence == 0 || requests[0].Sequence == 8 {
		t.Fatalf("application external requests = %+v", requests)
	}
	applicationToken := requests[0].Sequence
	if repeated := backend.ExternalRequests(); len(repeated) != 1 || repeated[0].Sequence != applicationToken {
		t.Fatalf("repeated application token = %+v, want %d", repeated, applicationToken)
	}
	requests[0].Data[0] = 9
	if application.requests[0].Data[0] != 1 {
		t.Fatal("routed external request aliased adapter-owned data")
	}

	if _, err := backend.Open(context.Background(), frontend.OpenRequest{Firmware: true}); err != nil {
		t.Fatal(err)
	}
	system.requests = []frontend.ExternalRequest{{
		Sequence: 8, Kind: frontend.ExternalRequestSMS,
	}}
	systemRequests := backend.ExternalRequests()
	if len(systemRequests) != 1 || systemRequests[0].Sequence == applicationToken {
		t.Fatalf("system external requests reused application token: %+v", systemRequests)
	}
	systemToken := systemRequests[0].Sequence

	backend.ClearExternalRequests(applicationToken)
	if len(application.cleared) != 1 || application.cleared[0] != 8 {
		t.Fatalf("application acknowledgements after switch = %v", application.cleared)
	}
	if len(system.cleared) != 0 {
		t.Fatalf("new adapter received old acknowledgement: %v", system.cleared)
	}

	backend.ClearExternalRequests(^uint64(0))
	if len(application.cleared) != 1 || len(system.cleared) != 0 {
		t.Fatalf("unobserved token was forwarded: application=%v system=%v",
			application.cleared, system.cleared)
	}

	backend.ClearExternalRequests(systemToken)
	if len(system.cleared) != 1 || system.cleared[0] != 8 {
		t.Fatalf("system acknowledgements = %v", system.cleared)
	}
}

func TestExternalRequestTokenCannotAcknowledgeReopenedAdapter(t *testing.T) {
	backend, application, _ := newTestBackend()
	application.requests = []frontend.ExternalRequest{{
		Sequence: 1, Kind: frontend.ExternalRequestBrowser,
	}}
	requests := backend.ExternalRequests()
	if len(requests) != 1 {
		t.Fatalf("initial external requests = %+v", requests)
	}
	staleToken := requests[0].Sequence

	if _, err := backend.Open(context.Background(), frontend.OpenRequest{}); err != nil {
		t.Fatal(err)
	}
	backend.ClearExternalRequests(staleToken)
	if len(application.cleared) != 0 {
		t.Fatalf("stale token reached reopened adapter: %v", application.cleared)
	}

	requests = backend.ExternalRequests()
	if len(requests) != 1 || requests[0].Sequence == staleToken {
		t.Fatalf("reopened adapter token = %+v, stale token %d", requests, staleToken)
	}
	backend.ClearExternalRequests(requests[0].Sequence)
	if len(application.cleared) != 1 || application.cleared[0] != 1 {
		t.Fatalf("current token acknowledgement = %v", application.cleared)
	}
}

func TestFailedOpenInvalidatesExternalRequestToken(t *testing.T) {
	backend, application, _ := newTestBackend()
	application.requests = []frontend.ExternalRequest{{
		Sequence: 4, Kind: frontend.ExternalRequestBrowser,
	}}
	requests := backend.ExternalRequests()
	if len(requests) != 1 {
		t.Fatalf("initial external requests = %+v", requests)
	}
	staleToken := requests[0].Sequence

	application.openErr = errors.New("machine replacement failed")
	if _, err := backend.Open(context.Background(), frontend.OpenRequest{}); err == nil {
		t.Fatal("synthetic open failure was not returned")
	}
	backend.ClearExternalRequests(staleToken)
	if len(application.cleared) != 0 {
		t.Fatalf("pre-open token survived failed replacement: %v", application.cleared)
	}

	requests = backend.ExternalRequests()
	if len(requests) != 1 || requests[0].Sequence == staleToken {
		t.Fatalf("post-failure external requests = %+v, stale token %d", requests, staleToken)
	}
}

func TestExternalRequestSnapshotPrunesDroppedSequences(t *testing.T) {
	backend, application, _ := newTestBackend()
	application.requests = []frontend.ExternalRequest{
		{Sequence: 1, Kind: frontend.ExternalRequestBrowser},
		{Sequence: 2, Kind: frontend.ExternalRequestBrowser},
	}
	initial := backend.ExternalRequests()
	if len(initial) != 2 {
		t.Fatalf("initial external requests = %+v", initial)
	}
	droppedToken := initial[0].Sequence
	retainedToken := initial[1].Sequence

	application.requests = application.requests[1:]
	remaining := backend.ExternalRequests()
	if len(remaining) != 1 || remaining[0].Sequence != retainedToken {
		t.Fatalf("remaining external requests = %+v, want token %d", remaining, retainedToken)
	}
	backend.ClearExternalRequests(droppedToken)
	if len(application.cleared) != 0 {
		t.Fatalf("dropped sequence token was forwarded: %v", application.cleared)
	}
	if len(backend.externalRequestTokens) != 1 || len(backend.externalRequestObservations) != 1 {
		t.Fatalf("live mappings after one drop = tokens %d observations %d",
			len(backend.externalRequestTokens), len(backend.externalRequestObservations))
	}

	application.requests = nil
	if requests := backend.ExternalRequests(); len(requests) != 0 {
		t.Fatalf("empty external requests = %+v", requests)
	}
	if len(backend.externalRequestTokens) != 0 || len(backend.externalRequestObservations) != 0 {
		t.Fatalf("live mappings after all drops = tokens %d observations %d",
			len(backend.externalRequestTokens), len(backend.externalRequestObservations))
	}
}

func TestConcurrentOpensAreSerialized(t *testing.T) {
	application := &blockingOpenAdapter{
		recordingAdapter: &recordingAdapter{name: "application"},
		started:          make(chan struct{}, 2),
		release:          make(chan struct{}, 2),
	}
	backend := newBackend(application, &recordingAdapter{name: "system"})
	results := make(chan error, 2)
	go func() {
		_, err := backend.Open(context.Background(), frontend.OpenRequest{Path: "first"})
		results <- err
	}()
	<-application.started
	go func() {
		_, err := backend.OpenWithProgress(
			context.Background(),
			frontend.OpenRequest{Path: "second"},
			nil,
		)
		results <- err
	}()
	select {
	case <-application.started:
		t.Fatal("second Open entered the adapter before the first completed")
	case <-time.After(50 * time.Millisecond):
	}
	application.release <- struct{}{}
	select {
	case <-application.started:
	case <-time.After(time.Second):
		t.Fatal("second Open did not start after the first completed")
	}
	application.release <- struct{}{}
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	application.mu.Lock()
	maxInFlight := application.maxInFlight
	application.mu.Unlock()
	if maxInFlight != 1 {
		t.Fatalf("maximum concurrent adapter Opens = %d, want 1", maxInFlight)
	}
}

func TestOpenWaitsForExternalRequestSnapshotWithoutDeadlock(t *testing.T) {
	application := &blockingSnapshotAdapter{
		recordingAdapter: &recordingAdapter{
			name: "application",
			requests: []frontend.ExternalRequest{{
				Sequence: 1, Kind: frontend.ExternalRequestBrowser,
			}},
		},
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	backend := newBackend(application, &recordingAdapter{name: "system"})
	snapshotResult := make(chan []frontend.ExternalRequest, 1)
	go func() { snapshotResult <- backend.ExternalRequests() }()
	<-application.started
	openResult := make(chan error, 1)
	go func() {
		_, err := backend.Open(context.Background(), frontend.OpenRequest{})
		openResult <- err
	}()
	select {
	case err := <-openResult:
		t.Fatalf("Open completed while snapshot held the routing lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(application.release)
	select {
	case requests := <-snapshotResult:
		if len(requests) != 1 {
			t.Fatalf("snapshot requests = %+v", requests)
		}
	case <-time.After(time.Second):
		t.Fatal("snapshot did not finish after release")
	}
	select {
	case err := <-openResult:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Open deadlocked behind completed snapshot")
	}
}

func TestOpenWaitsForExternalRequestAcknowledgementWithoutDeadlock(t *testing.T) {
	application := &blockingClearAdapter{
		recordingAdapter: &recordingAdapter{
			name: "application",
			requests: []frontend.ExternalRequest{{
				Sequence: 1, Kind: frontend.ExternalRequestBrowser,
			}},
		},
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	backend := newBackend(application, &recordingAdapter{name: "system"})
	requests := backend.ExternalRequests()
	if len(requests) != 1 {
		t.Fatalf("external requests = %+v", requests)
	}
	clearDone := make(chan struct{})
	go func() {
		backend.ClearExternalRequests(requests[0].Sequence)
		close(clearDone)
	}()
	<-application.started
	openResult := make(chan error, 1)
	go func() {
		_, err := backend.OpenWithProgress(
			context.Background(),
			frontend.OpenRequest{},
			nil,
		)
		openResult <- err
	}()
	select {
	case err := <-openResult:
		t.Fatalf("OpenWithProgress completed while ACK held the routing lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(application.release)
	select {
	case <-clearDone:
	case <-time.After(time.Second):
		t.Fatal("acknowledgement did not finish after release")
	}
	select {
	case err := <-openResult:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("OpenWithProgress deadlocked behind completed acknowledgement")
	}
}

func TestCloseWaitsForOpenAndInvalidatesPublishedToken(t *testing.T) {
	application := &blockingOpenAdapter{
		recordingAdapter: &recordingAdapter{name: "application"},
		started:          make(chan struct{}, 1),
		release:          make(chan struct{}, 1),
	}
	backend := newBackend(application, &recordingAdapter{name: "system"})
	openResult := make(chan error, 1)
	go func() {
		_, err := backend.Open(context.Background(), frontend.OpenRequest{})
		openResult <- err
	}()
	<-application.started
	closeResult := make(chan error, 1)
	go func() { closeResult <- backend.Close() }()
	select {
	case err := <-closeResult:
		t.Fatalf("Close completed during Open: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	application.release <- struct{}{}
	if err := <-openResult; err != nil {
		t.Fatal(err)
	}
	if err := <-closeResult; err != nil {
		t.Fatal(err)
	}
	if application.closes != 1 {
		t.Fatalf("adapter closes = %d, want 1", application.closes)
	}
}

func TestFailedCloseInvalidatesExternalRequestToken(t *testing.T) {
	backend, application, _ := newTestBackend()
	application.requests = []frontend.ExternalRequest{{
		Sequence: 9, Kind: frontend.ExternalRequestBrowser,
	}}
	requests := backend.ExternalRequests()
	if len(requests) != 1 {
		t.Fatalf("initial external requests = %+v", requests)
	}
	staleToken := requests[0].Sequence
	application.closeErr = errors.New("save failed")
	if err := backend.Close(); err == nil {
		t.Fatal("synthetic close failure was not returned")
	}
	backend.ClearExternalRequests(staleToken)
	if len(application.cleared) != 0 {
		t.Fatalf("pre-close token survived failed close: %v", application.cleared)
	}
	requests = backend.ExternalRequests()
	if len(requests) != 1 || requests[0].Sequence == staleToken {
		t.Fatalf("post-close-failure requests = %+v, stale token %d", requests, staleToken)
	}
}

func TestExternalRequestTokenWrapSkipsZeroAndLiveToken(t *testing.T) {
	backend, application, _ := newTestBackend()
	application.requests = []frontend.ExternalRequest{{
		Sequence: 10, Kind: frontend.ExternalRequestBrowser,
	}}
	initial := backend.ExternalRequests()
	if len(initial) != 1 || initial[0].Sequence != 1 {
		t.Fatalf("initial external requests = %+v", initial)
	}
	backend.nextExternalRequestSequence = math.MaxUint64 - 1
	application.requests = append(application.requests,
		frontend.ExternalRequest{Sequence: 11, Kind: frontend.ExternalRequestBrowser})
	wrapped := backend.ExternalRequests()
	if len(wrapped) != 2 || wrapped[1].Sequence != math.MaxUint64 {
		t.Fatalf("pre-wrap external requests = %+v", wrapped)
	}
	application.requests = append(application.requests,
		frontend.ExternalRequest{Sequence: 12, Kind: frontend.ExternalRequestBrowser})
	wrapped = backend.ExternalRequests()
	if len(wrapped) != 3 || wrapped[2].Sequence != 2 {
		t.Fatalf("wrapped external requests = %+v, want token 2 after skipping 0 and live 1", wrapped)
	}
}

func TestExternalRequestPruningFollowsDisorderedOutbox(t *testing.T) {
	backend, application, _ := newTestBackend()
	application.requests = []frontend.ExternalRequest{
		{Sequence: 10, Kind: frontend.ExternalRequestBrowser},
		{Sequence: 20, Kind: frontend.ExternalRequestBrowser},
		{Sequence: 5, Kind: frontend.ExternalRequestBrowser},
	}
	requests := backend.ExternalRequests()
	if len(requests) != 3 {
		t.Fatalf("disordered external requests = %+v", requests)
	}
	lateLowToken := requests[2].Sequence
	backend.ClearExternalRequests(requests[0].Sequence)
	if len(application.requests) != 2 || application.requests[1].Sequence != 5 {
		t.Fatalf("source outbox after clearing 10 = %+v", application.requests)
	}
	backend.ClearExternalRequests(lateLowToken)
	if len(application.cleared) != 2 || application.cleared[1] != 5 {
		t.Fatalf("late low-sequence acknowledgement = %v", application.cleared)
	}
}

// Switching halves must release the input the outgoing half holds. For the
// whole-phone adapter that close is also what commits the writable NAND, so
// leaving it open would lose a session.
func TestSwitchingAdaptersClosesTheOutgoingOne(t *testing.T) {
	backend, application, system := newTestBackend()
	ctx := context.Background()

	if _, err := backend.Open(ctx, frontend.OpenRequest{Path: "title.zip"}); err != nil {
		t.Fatal(err)
	}
	if application.closes != 0 {
		t.Fatalf("application was closed %d times before any switch", application.closes)
	}
	if _, err := backend.Open(ctx, frontend.OpenRequest{Firmware: true}); err != nil {
		t.Fatal(err)
	}
	if application.closes != 1 {
		t.Fatalf("application closes after switching to firmware = %d, want 1", application.closes)
	}
	if _, err := backend.Open(ctx, frontend.OpenRequest{Path: "other.zip"}); err != nil {
		t.Fatal(err)
	}
	if system.closes != 1 {
		t.Fatalf("system closes after switching back = %d, want 1", system.closes)
	}
}

func TestSwitchIsAbandonedWhenTheOutgoingAdapterFailsToClose(t *testing.T) {
	backend, application, system := newTestBackend()
	application.requests = []frontend.ExternalRequest{{
		Sequence: 6, Kind: frontend.ExternalRequestBrowser,
	}}
	requests := backend.ExternalRequests()
	if len(requests) != 1 {
		t.Fatalf("application external requests = %+v", requests)
	}
	applicationToken := requests[0].Sequence
	application.closeErr = errors.New("still writing save data")

	_, err := backend.Open(context.Background(), frontend.OpenRequest{Firmware: true})
	if err == nil {
		t.Fatal("a failed close still allowed the switch")
	}
	if len(system.opens) != 0 {
		t.Fatalf("system adapter was opened %d times despite the failed close", len(system.opens))
	}
	if backend.BackendName() != "application" {
		t.Fatalf("active adapter = %q after a failed close, want the outgoing one", backend.BackendName())
	}
	backend.ClearExternalRequests(applicationToken)
	if len(application.cleared) != 1 || application.cleared[0] != 6 {
		t.Fatalf("outgoing adapter token was lost after failed switch: %v", application.cleared)
	}
	if len(system.cleared) != 0 {
		t.Fatalf("failed switch routed acknowledgement to incoming adapter: %v", system.cleared)
	}
}

// The shell configures audio, font, and CPU once at startup. A half that
// becomes active later never saw those calls, so the router has to replay them.
func TestRememberedSettingsAreReappliedOnSwitch(t *testing.T) {
	backend, application, system := newTestBackend()
	ctx := context.Background()

	if err := backend.ConfigureAudio(frontend.AudioSettings{}); err != nil {
		t.Fatal(err)
	}
	if err := backend.ConfigureFont(frontend.FontSettings{Name: "galmuri9"}); err != nil {
		t.Fatal(err)
	}
	if err := backend.ConfigureCPU(frontend.CPUSettings{Name: "precise"}); err != nil {
		t.Fatal(err)
	}
	if len(application.audio) != 1 || len(application.fonts) != 1 || len(application.cpus) != 1 {
		t.Fatalf("startup settings did not reach the active adapter: %+v", application)
	}

	if _, err := backend.Open(ctx, frontend.OpenRequest{Firmware: true}); err != nil {
		t.Fatal(err)
	}
	if len(system.audio) != 1 || len(system.fonts) != 1 || len(system.cpus) != 1 {
		t.Fatalf("settings were not replayed onto the incoming adapter: %+v", system)
	}
	if got, want := system.fonts[0].Name, "galmuri9"; got != want {
		t.Fatalf("replayed font = %q, want %q", got, want)
	}
	if got, want := system.cpus[0].Name, "precise"; got != want {
		t.Fatalf("replayed CPU core = %q, want %q", got, want)
	}
}

// A half that does not implement an optional interface must degrade rather
// than reach for a method that is not there.
func TestOptionalContractFallsBackForAPlainAdapter(t *testing.T) {
	application := &recordingAdapter{name: "application"}
	system := &plainAdapter{recordingAdapter{name: "system"}}
	backend := newBackend(application, frontend.Backend(struct{ frontend.Backend }{system}))
	ctx := context.Background()

	if _, err := backend.Open(ctx, frontend.OpenRequest{Firmware: true}); err != nil {
		t.Fatal(err)
	}
	if err := backend.ConfigureAudio(frontend.AudioSettings{}); err != nil {
		t.Fatalf("ConfigureAudio on a plain adapter = %v, want nil", err)
	}
	if err := backend.RunFrame(ctx); err != nil {
		t.Fatalf("RunFrame on a plain adapter = %v, want nil", err)
	}
	if got := backend.AvailableCPUBackends(); got != nil {
		t.Fatalf("AvailableCPUBackends on a plain adapter = %v, want nil", got)
	}
	if _, err := backend.ExecuteToolAction(ctx, frontend.ToolRequest{}); err == nil {
		t.Fatal("ExecuteToolAction on a plain adapter reported success")
	}
}

func TestFirmwareCapabilityIsAdvertised(t *testing.T) {
	backend, _, _ := newTestBackend()
	if !backend.SupportsFirmware() {
		t.Fatal("the routed backend did not advertise firmware support")
	}
}

// saveTransferAdapter is a half that offers the optional save backup contract,
// so routing to it can be told apart from the absent-capability fallback.
type saveTransferAdapter struct {
	recordingAdapter
	exported []byte
	imported []byte
}

func (a *saveTransferAdapter) ExportSaveData() ([]byte, error) { return a.exported, nil }

func (a *saveTransferAdapter) ImportSaveData(data []byte) error {
	a.imported = data
	return nil
}

func TestSaveTransferRoutesToActiveAdapter(t *testing.T) {
	application := &saveTransferAdapter{
		recordingAdapter: recordingAdapter{name: "application"},
		exported:         []byte("blob"),
	}
	system := &recordingAdapter{name: "system"}
	backend := newBackend(application, system)

	blob, err := backend.ExportSaveData()
	if err != nil || string(blob) != "blob" {
		t.Fatalf("export routed to the wrong adapter: %q %v", blob, err)
	}
	if err := backend.ImportSaveData([]byte("restore")); err != nil {
		t.Fatalf("import: %v", err)
	}
	if string(application.imported) != "restore" {
		t.Fatalf("import reached the adapter as %q", application.imported)
	}

	// The whole-phone adapter offers no per-title save backup, so switching to
	// it must report the capability as absent rather than pretend to back up.
	if _, err := backend.selectAdapter(true); err != nil {
		t.Fatalf("select system adapter: %v", err)
	}
	if _, err := backend.ExportSaveData(); err == nil {
		t.Fatal("the firmware adapter claimed save backup support")
	}
	if err := backend.ImportSaveData([]byte("x")); err == nil {
		t.Fatal("the firmware adapter claimed save restore support")
	}
}

func TestIconForwardsToApplicationAdapter(t *testing.T) {
	application := &recordingAdapter{name: "app"}
	system := &recordingAdapter{name: "system"}
	backend := newBackend(application, system)

	// Switch the active adapter to system; an icon is path-keyed metadata and
	// must still be answered by the application adapter.
	if _, err := backend.Open(context.Background(), frontend.OpenRequest{Firmware: true}); err != nil {
		t.Fatal(err)
	}

	data, err := backend.Icon("game.jar")
	if err != nil {
		t.Fatalf("Icon error: %v", err)
	}
	if string(data) != "icon:app" {
		t.Fatalf("Icon answered by %q, want the application adapter", data)
	}
	if len(application.icons) != 1 || application.icons[0] != "game.jar" {
		t.Fatalf("application icon requests = %v", application.icons)
	}
	if len(system.icons) != 0 {
		t.Fatal("icon request reached the system adapter")
	}
}
