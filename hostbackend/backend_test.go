package hostbackend

import (
	"context"
	"errors"
	"testing"

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
	closeErr error
	icons    []string
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
	return frontend.InputInfo{DisplayName: a.name}, nil
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

func (a *recordingAdapter) BackendName() string { return a.name }

// plainAdapter implements only the required contract, so the forwarding paths
// that depend on an optional interface can be checked for their fallbacks.
type plainAdapter struct{ recordingAdapter }

func (a *plainAdapter) ConfigureAudio(frontend.AudioSettings) error { panic("not implemented") }
func (a *plainAdapter) ConfigureFont(frontend.FontSettings) error   { panic("not implemented") }
func (a *plainAdapter) ConfigureCPU(frontend.CPUSettings) error     { panic("not implemented") }
func (a *plainAdapter) AvailableCPUBackends() []string              { panic("not implemented") }
func (a *plainAdapter) RunFrame(context.Context) error              { panic("not implemented") }

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
