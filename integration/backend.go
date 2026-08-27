package integration

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"image"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/mirusu400/aram-core/application"
	"github.com/mirusu400/aram-core/cheat"
	aramcore "github.com/mirusu400/aram-core/core"
	"github.com/mirusu400/aram-core/cpu"
	"github.com/mirusu400/aram-core/loader"
	"github.com/mirusu400/aram-core/runtime"
	"github.com/mirusu400/aram-frontend/frontend"
)

const (
	stateSchemaVersion = 1
	maxStateHeaderSize = 64 * 1024
)

var stateMagic = []byte("ARAMSTATE\x00")

type Backend struct {
	operationMu   sync.Mutex
	mu            sync.RWMutex
	factory       aramcore.Factory
	machine       aramcore.Machine
	sourceFile    *os.File
	source        aramcore.Source
	input         frontend.InputInfo
	stateRoot     string
	audio         frontend.AudioSettings
	fontChoice    string
	cpuChoice     string
	runRequested  bool
	lastFrameHash uint64
	// lastFramePixels retains the last published frame so the next one can be
	// compared against it exactly instead of hashed.
	lastFramePixels []byte
	// lastPresentation is the core presentation sequence the published frame
	// came from, for backends that report one.
	lastPresentation uint64
	frameSequence    uint64

	imageSHA256        string
	cheatStore         *cheatCatalogStore
	cheats             *cheat.Library
	cheatUnavailable   string
	cheatImported      bool
	cheatCatalogSource string
	cheatIdentity      string
	cheatApplyWarning  string
}

func NewBackend(factory aramcore.Factory) *Backend {
	if factory == nil {
		defaultFactory := application.NewFactory()
		defaultFactory.FrameRunBudget = application.DefaultHandsetRunBudget
		defaultFactory.KTFRunBudget = application.DefaultKTFHandsetRunBudget
		factory = defaultFactory
	}
	return &Backend{factory: factory, cheatStore: newCheatCatalogStore()}
}

func (backend *Backend) BackendName() string {
	return "aram-core"
}

func (backend *Backend) Open(
	ctx context.Context,
	request frontend.OpenRequest,
) (frontend.InputInfo, error) {
	return backend.OpenWithProgress(ctx, request, nil)
}

func (backend *Backend) OpenWithProgress(
	ctx context.Context,
	request frontend.OpenRequest,
	progress func(frontend.OpenStage),
) (frontend.InputInfo, error) {
	backend.operationMu.Lock()
	defer backend.operationMu.Unlock()

	if progress != nil {
		progress(frontend.OpenStageInspecting)
	}
	if request.Firmware {
		return frontend.InputInfo{
				DisplayName: displayName(request),
				Format:      "firmware",
			}, backendError(
				frontend.FailureUnsupportedProfile,
				errors.New("system-mode firmware loading is not implemented"),
			)
	}
	source, info, sourceFile, err := inspectSource(request)
	if err != nil {
		return info, backendError(frontend.FailureMalformedInput, err)
	}

	if progress != nil {
		progress(frontend.OpenStageLoading)
	}
	machine, err := backend.factoryForCreate().Create(ctx, source)
	if err != nil {
		_ = sourceFile.Close()
		return info, backendError(classifyFactoryError(err, source.Format), err)
	}
	if machine == nil {
		_ = sourceFile.Close()
		return info, backendError(
			frontend.FailureBackendUnavailable,
			errors.New("core factory returned no machine"),
		)
	}
	if machine.State() == aramcore.StateEmpty {
		if err := machine.Load(ctx, source); err != nil {
			_ = machine.Close()
			_ = sourceFile.Close()
			return info, backendError(classifyMachineError(machine, err), err)
		}
	}
	var imageSHA256 string
	if provider, ok := machine.(interface {
		ImageInfo() application.ImageInfo
	}); ok {
		imageInfo := provider.ImageInfo()
		info.Format = string(imageInfo.SourceKind)
		info.ProfileID = imageInfo.ProfileID
		source.ProfileID = imageInfo.ProfileID
		imageSHA256 = imageInfo.ImageSHA256
		info.ImageSHA256 = imageSHA256
	}
	// Wrapping happens before the machine is published so every later command
	// goes through the wrapper that serializes cheats with guest execution.
	machine, library, cheatUnavailable := attachCheats(machine)

	backend.mu.Lock()
	oldMachine := backend.machine
	oldFile := backend.sourceFile
	backend.machine = machine
	backend.sourceFile = sourceFile
	backend.source = source
	backend.input = info
	backend.runRequested = false
	backend.lastFrameHash = 0
	backend.lastFramePixels = nil
	backend.lastPresentation = 0
	backend.frameSequence = 0
	backend.cheats = library
	backend.cheatUnavailable = cheatUnavailable
	backend.cheatImported = false
	backend.cheatCatalogSource = ""
	backend.cheatIdentity = ""
	backend.cheatApplyWarning = ""
	backend.imageSHA256 = imageSHA256
	backend.mu.Unlock()

	if oldMachine != nil {
		_ = oldMachine.Close()
	}
	if oldFile != nil {
		_ = oldFile.Close()
	}

	// Load the title's persisted writable storage before the shell starts the
	// machine, so a Clet's first-run save (for example 에픽크로니클PE's gopt.sav)
	// survives the exit-and-relaunch its "restart required" notice demands.
	backend.restoreSaveData(machine, info.SHA256)

	// Catalog defaults must be in guest memory before Open returns, because
	// the shell starts the machine immediately afterwards and a repair such
	// as skipping a dead authentication server patches code the guest runs
	// while booting. Waiting for the Cheat Manager to import the catalog
	// left default-enabled cheats inert until the first reset. A title with
	// no published document or an unreachable database is an ordinary open;
	// the panel retries and reports when asked.
	if library != nil {
		ensureCtx, cancel := context.WithTimeout(ctx, openCheatEnsureTimeout)
		_, _ = backend.ensureCheatCatalog(ensureCtx, library, false)
		cancel()
	}
	return info, nil
}

func inspectSource(
	request frontend.OpenRequest,
) (aramcore.Source, frontend.InputInfo, *os.File, error) {
	// A host without a readable filesystem path (the web/wasm build) hands the
	// input in-band as bytes. Inspect and load from memory, with no os access.
	if len(request.Data) > 0 {
		return inspectSourceBytes(request)
	}
	if request.Path == "" {
		return aramcore.Source{}, frontend.InputInfo{
			DisplayName: request.DisplayName,
		}, nil, errors.New("the selected document has no backend-readable path or handle")
	}

	fileInfo, err := os.Stat(request.Path)
	if err != nil {
		return aramcore.Source{}, frontend.InputInfo{
			DisplayName: displayName(request),
		}, nil, err
	}
	if fileInfo.IsDir() {
		return aramcore.Source{}, frontend.InputInfo{
			DisplayName: displayName(request),
			Format:      "firmware-directory",
		}, nil, errors.New("firmware directory machines are not implemented by aram-core")
	}

	report, err := loader.InspectFile(request.Path)
	if err != nil {
		return aramcore.Source{}, frontend.InputInfo{
			DisplayName: displayName(request),
		}, nil, err
	}
	sourceFile, err := os.Open(request.Path)
	if err != nil {
		return aramcore.Source{}, frontend.InputInfo{
			DisplayName: displayName(request),
			Format:      string(report.Kind),
			Size:        report.Size,
			SHA256:      report.SHA256,
		}, nil, err
	}
	name := request.DisplayName
	if name == "" {
		name = filepath.Base(report.Path)
	}
	source := aramcore.Source{
		Name:     name,
		Path:     report.Path,
		Format:   string(report.Kind),
		SHA256:   report.SHA256,
		ReaderAt: sourceFile,
		Size:     report.Size,
	}
	info := frontend.InputInfo{
		DisplayName: name,
		Format:      string(report.Kind),
		Size:        report.Size,
		SHA256:      report.SHA256,
	}
	return source, info, sourceFile, nil
}

// inspectSourceBytes is the in-memory counterpart of inspectSource for hosts
// that deliver the input as bytes rather than a filesystem path (web/wasm). It
// inspects and hashes the bytes through the same loader used for files, then
// backs the aramcore.Source with a bytes.Reader. It returns a nil *os.File
// because there is no descriptor to own; callers guard the returned file before
// closing it.
func inspectSourceBytes(
	request frontend.OpenRequest,
) (aramcore.Source, frontend.InputInfo, *os.File, error) {
	name := request.DisplayName
	if name == "" {
		name = "input"
	}
	report, err := loader.InspectBytes(name, request.Data)
	if err != nil {
		return aramcore.Source{}, frontend.InputInfo{
			DisplayName: name,
		}, nil, err
	}
	source := aramcore.Source{
		Name:     name,
		Format:   string(report.Kind),
		SHA256:   report.SHA256,
		ReaderAt: bytes.NewReader(request.Data),
		Size:     report.Size,
	}
	info := frontend.InputInfo{
		DisplayName: name,
		Format:      string(report.Kind),
		Size:        report.Size,
		SHA256:      report.SHA256,
	}
	return source, info, nil, nil
}

func (backend *Backend) State() frontend.BackendState {
	backend.mu.RLock()
	machine := backend.machine
	runRequested := backend.runRequested
	backend.mu.RUnlock()
	if machine == nil {
		return frontend.StateEmpty
	}
	state := machine.State()
	if runRequested {
		switch state {
		case aramcore.StateReady, aramcore.StateRunning, aramcore.StatePaused:
			return frontend.StateRunning
		}
	}
	switch state {
	case aramcore.StateReady:
		return frontend.StateReady
	case aramcore.StateRunning:
		return frontend.StateRunning
	case aramcore.StatePaused:
		return frontend.StatePaused
	case aramcore.StateStopped:
		return frontend.StateStopped
	case aramcore.StateFaulted:
		return frontend.StateFaulted
	default:
		return frontend.StateEmpty
	}
}

func (backend *Backend) Supports(command frontend.BackendCommand) bool {
	return backend.Capability(command).Supported
}

func (backend *Backend) Capability(command frontend.BackendCommand) frontend.Capability {
	machine := backend.currentMachine()
	if machine == nil {
		return frontend.Capability{Reason: "No aram-core machine is loaded"}
	}
	state := backend.State()
	supported := false
	switch command {
	case frontend.CommandStart:
		// Stopped is startable too: a title that exited on its own (a Clet's
		// MC_knlExit) restarts by re-bootstrapping on Start, so the user can
		// relaunch it without a separate Reset.
		supported = state == frontend.StateReady ||
			state == frontend.StatePaused ||
			state == frontend.StateStopped
	case frontend.CommandPauseResume:
		supported = state == frontend.StateRunning || state == frontend.StatePaused
	case frontend.CommandStop:
		supported = state == frontend.StateRunning || state == frontend.StatePaused
	case frontend.CommandReset:
		supported = state != frontend.StateEmpty && state != frontend.StateFaulted
	case frontend.CommandFrame:
		supported = state == frontend.StateReady ||
			state == frontend.StatePaused
	case frontend.CommandLoadState, frontend.CommandSaveState:
		supported = state != frontend.StateEmpty && state != frontend.StateRunning
	case frontend.CommandFastForward:
		return frontend.Capability{
			Reason: "The core machine contract does not expose speed control yet",
		}
	case frontend.CommandRewind:
		return frontend.Capability{
			Reason: "The core machine contract does not expose rewind history yet",
		}
	default:
		return frontend.Capability{Reason: "Unknown backend command"}
	}
	if !supported {
		return frontend.Capability{
			Reason: fmt.Sprintf("%s is unavailable while the machine is %s", command, state),
		}
	}
	return frontend.Capability{Supported: true}
}

func (backend *Backend) Execute(
	ctx context.Context,
	command frontend.BackendCommand,
) error {
	return backend.ExecuteCommand(ctx, frontend.CommandRequest{
		Command: command,
		Slot:    0,
		Speed:   1,
	})
}

func (backend *Backend) ExecuteCommand(
	ctx context.Context,
	request frontend.CommandRequest,
) error {
	backend.operationMu.Lock()
	defer backend.operationMu.Unlock()

	capability := backend.Capability(request.Command)
	if !capability.Supported {
		return errors.New(capability.Reason)
	}
	machine := backend.currentMachine()
	if machine == nil {
		return backendError(
			frontend.FailureBackendUnavailable,
			errors.New("no aram-core machine is loaded"),
		)
	}

	var err error
	switch request.Command {
	case frontend.CommandStart:
		if machine.State() == aramcore.StateStopped {
			// The guest ended (for example a first-run Clet's MC_knlExit).
			// Re-bootstrap it — preserving the title's writable storage — so
			// Start restarts an exited title instead of doing nothing. The
			// reload reads the save the exited run wrote (에픽크로니클PE's
			// gopt.sav), which is exactly what its "restart required" notice
			// asks for.
			if resetErr := machine.Reset(ctx); resetErr != nil {
				err = resetErr
				break
			}
		}
		err = machine.Start(ctx)
		if err == nil {
			backend.setRunRequested(machineCanContinue(machine.State()))
		}
	case frontend.CommandPauseResume:
		if backend.runningRequested() {
			backend.setRunRequested(false)
		} else {
			backend.setRunRequested(machineCanContinue(machine.State()))
		}
	case frontend.CommandStop:
		backend.setRunRequested(false)
		err = machine.Stop()
		if err == nil {
			backend.persistSaveData(machine, backend.currentInputHash())
		}
	case frontend.CommandReset:
		backend.setRunRequested(false)
		// Capture the current writable storage before the reset re-bootstraps
		// the guest, so a save written this run is not lost on restart.
		backend.persistSaveData(machine, backend.currentInputHash())
		err = machine.Reset(ctx)
	case frontend.CommandFrame:
		err = machine.StepFrame(ctx)
	case frontend.CommandSaveState:
		err = backend.saveState(request.Slot)
	case frontend.CommandLoadState:
		backend.setRunRequested(false)
		err = backend.loadState(request.Slot)
	default:
		err = fmt.Errorf("unsupported backend command %q", request.Command)
	}
	if err != nil {
		return backendError(classifyMachineError(machine, err), err)
	}
	return nil
}

// RunFrame advances one guest presentation quantum while preserving the
// product-level running intent across the core's deterministic frame yield.
func (backend *Backend) RunFrame(ctx context.Context) error {
	backend.operationMu.Lock()
	defer backend.operationMu.Unlock()

	if !backend.runningRequested() {
		return nil
	}
	machine := backend.currentMachine()
	if machine == nil {
		backend.setRunRequested(false)
		return backendError(
			frontend.FailureBackendUnavailable,
			errors.New("no aram-core machine is loaded"),
		)
	}
	if err := machine.StepFrame(ctx); err != nil {
		backend.setRunRequested(false)
		return backendError(classifyMachineError(machine, err), err)
	}
	if !machineCanContinue(machine.State()) {
		backend.setRunRequested(false)
		// The guest ended (for example a Clet called MC_knlExit); flush its
		// writable storage so the next launch reloads the save.
		backend.persistSaveData(machine, backend.currentInputHash())
	}
	return nil
}

func (backend *Backend) VideoFrame() frontend.VideoFrame {
	machine := backend.currentMachine()
	if machine == nil {
		return frontend.VideoFrame{}
	}
	unwrapped := unwrapMachine(machine)
	if presenter, ok := unwrapped.(coreVideoPresenter); ok {
		return backend.presentedTimedVideoFrame(presenter)
	}
	if presenter, ok := unwrapped.(coreFramePresenter); ok {
		return backend.presentedVideoFrame(presenter)
	}
	frame := machine.Framebuffer()
	if frame == nil || frame.Bounds().Dx() <= 0 || frame.Bounds().Dy() <= 0 {
		return frontend.VideoFrame{}
	}
	backend.mu.Lock()
	// frameChanged always runs, even for the first frame, so the comparison
	// baseline is recorded before the next tick reads it.
	changed := backend.frameChanged(frame)
	if backend.frameSequence == 0 || changed {
		backend.frameSequence++
	}
	sequence := backend.frameSequence
	backend.mu.Unlock()
	return frontend.VideoFrame{Image: frame, Sequence: sequence}
}

func (backend *Backend) QueueInput(event frontend.InputEvent) error {
	machine := backend.currentMachine()
	if machine == nil {
		return backendError(
			frontend.FailureBackendUnavailable,
			errors.New("no aram-core machine is loaded"),
		)
	}
	return machine.QueueInput(aramcore.InputEvent{
		Control: event.Control,
		Pressed: event.Pressed,
		At:      event.At,
	})
}

func (backend *Backend) ConfigureAudio(settings frontend.AudioSettings) error {
	backend.mu.Lock()
	backend.audio = settings
	backend.mu.Unlock()
	// The mix-mode policy applies live so an A/B comparison does not require
	// reopening the title; a machine that predates this method simply misses the
	// live update and picks the policy up when next created.
	if machine := backend.currentMachine(); machine != nil {
		if setter, ok := machine.(interface{ SetAudioMixMode(bool) }); ok {
			setter.SetAudioMixMode(settings.MixMode)
		}
	}
	return nil
}

// Backend serves the optional font-selection interface so the shell can switch
// the handset fallback font.
var _ frontend.FontBackend = (*Backend)(nil)

// ConfigureFont records the handset fallback font selection. A built-in name is
// stored directly; a user-supplied font file is built and registered once, and
// its content-addressed name is stored instead. The choice is baked into the
// core machine at creation, so it takes effect the next time a title is opened.
func (backend *Backend) ConfigureFont(settings frontend.FontSettings) error {
	name := settings.Name
	if len(settings.Data) > 0 {
		registered, err := runtime.RegisterHandsetFont(settings.Data)
		if err != nil {
			return err
		}
		name = registered
	}
	backend.mu.Lock()
	backend.fontChoice = name
	backend.mu.Unlock()
	return nil
}

// ConfigureCPU records the CPU backend selection. An empty name keeps the
// default (the ARAM_CPU environment selection, or the precise interpreter); a
// known registered name is stored and baked into the next machine created, so
// it takes effect the next time a title is opened. An unknown name is rejected.
func (backend *Backend) ConfigureCPU(settings frontend.CPUSettings) error {
	name := settings.Name
	if name != "" {
		if _, err := application.ResolveCPUBackend(name); err != nil {
			return err
		}
	}
	backend.mu.Lock()
	backend.cpuChoice = name
	backend.mu.Unlock()
	return nil
}

// AvailableCPUBackends lists the selectable CPU backend names for the settings
// dropdown. Only the precise interpreter is present until a fast/native core
// registers itself.
func (backend *Backend) AvailableCPUBackends() []string {
	return application.CPUBackendNames()
}

// factoryForCreate returns the factory used to build the next machine with the
// current fallback-font selection applied. The default application.Factory is a
// value type, so setting the field on the returned copy never mutates the
// shared factory; injected factories that are not application.Factory are used
// unchanged.
func (backend *Backend) factoryForCreate() aramcore.Factory {
	backend.mu.RLock()
	factory := backend.factory
	fontChoice := backend.fontChoice
	cpuChoice := backend.cpuChoice
	audioMixMode := backend.audio.MixMode
	backend.mu.RUnlock()
	if concrete, ok := factory.(application.Factory); ok {
		concrete.FallbackFont = fontChoice
		concrete.AudioMixMode = audioMixMode
		// A frontend CPU selection overrides the factory default (which already
		// reflects the ARAM_CPU environment); an unknown name is ignored so the
		// default core still runs.
		if cpuChoice != "" {
			if newCPU, err := application.ResolveCPUBackend(cpuChoice); err == nil {
				concrete.NewCPU = newCPU
			}
		}
		return concrete
	}
	return factory
}

func (backend *Backend) DrainAudio() frontend.AudioChunk {
	machine := backend.currentMachine()
	if machine == nil {
		return frontend.AudioChunk{}
	}
	// Application machines publish immutable PCM under a dedicated lock at a
	// service-advance commit point. This capability is safe to drain while a
	// slow StepFrame still owns operationMu; older machines retain the serialized
	// fallback below.
	if publisher, ok := unwrapMachine(machine).(interface {
		DrainPublishedAudio() aramcore.AudioChunk
	}); ok {
		return frontendAudioChunk(publisher.DrainPublishedAudio())
	}
	if !backend.operationMu.TryLock() {
		return frontend.AudioChunk{}
	}
	defer backend.operationMu.Unlock()

	machine = backend.currentMachine()
	if machine == nil {
		return frontend.AudioChunk{}
	}
	return frontendAudioChunk(machine.DrainAudio())
}

func frontendAudioChunk(chunk aramcore.AudioChunk) frontend.AudioChunk {
	return frontend.AudioChunk{
		SampleRate:   chunk.SampleRate,
		Channels:     chunk.Channels,
		PCM16:        chunk.PCM16,
		StartGuestNS: chunk.StartGuestNS,
		StartSample:  chunk.StartSample,
		Generation:   chunk.Generation,
	}
}

func (backend *Backend) ToolSnapshot(
	ctx context.Context,
	kind frontend.ToolKind,
) (frontend.ToolSnapshot, error) {
	switch kind {
	case frontend.ToolCompatibility:
		backend.mu.RLock()
		input := backend.input
		source := backend.source
		image := backend.imageSHA256
		backend.mu.RUnlock()
		return frontend.ToolSnapshot{
			Title: "Compatibility Report",
			Lines: []string{
				"Input: " + input.DisplayName,
				"Format: " + input.Format,
				"File SHA-256: " + input.SHA256,
				// The image identity is what cheat catalogs are keyed on, so
				// it belongs where a reporter can copy it.
				"Image SHA-256: " + emptyFallback(image, "unavailable"),
				"Profile: " + emptyFallback(input.ProfileID, "unselected"),
				"Core source: " + source.Path,
			},
		}, nil
	case frontend.ToolCheats:
		return backend.cheatSnapshot(ctx, false, "")
	case frontend.ToolDebugger:
		machine := backend.currentMachine()
		if machine == nil {
			return frontend.ToolSnapshot{}, backendError(
				frontend.FailureBackendUnavailable,
				errors.New("no aram-core machine is loaded"),
			)
		}
		lines := []string{
			"CPU backend: portable interpreter",
			"State: " + machine.State().String(),
		}
		if provider, ok := unwrapMachine(machine).(interface {
			ImageInfo() application.ImageInfo
		}); ok {
			info := provider.ImageInfo()
			lines = append(lines,
				"Image: "+info.Name,
				fmt.Sprintf("Entry: 0x%08x (%s)", info.EntryPoint, modeName(info.Mode)),
			)
		}
		return frontend.ToolSnapshot{
			Title: "Debugger",
			Lines: lines,
		}, nil
	default:
		return frontend.ToolSnapshot{}, fmt.Errorf(
			"aram-core does not expose a checked %s service yet",
			kind,
		)
	}
}

func (backend *Backend) ExecuteToolAction(
	ctx context.Context,
	request frontend.ToolRequest,
) (frontend.ToolSnapshot, error) {
	switch request.Kind {
	case frontend.ToolCheats:
		return backend.executeCheatAction(ctx, request)
	default:
		return frontend.ToolSnapshot{}, fmt.Errorf(
			"aram-core does not expose %s actions yet",
			request.Kind,
		)
	}
}

func (backend *Backend) Close() error {
	backend.operationMu.Lock()
	defer backend.operationMu.Unlock()

	backend.mu.Lock()
	machine := backend.machine
	sourceFile := backend.sourceFile
	closingHash := backend.input.SHA256
	backend.machine = nil
	backend.sourceFile = nil
	backend.source = aramcore.Source{}
	backend.input = frontend.InputInfo{}
	backend.runRequested = false
	backend.lastFrameHash = 0
	backend.lastFramePixels = nil
	backend.lastPresentation = 0
	backend.frameSequence = 0
	backend.cheats = nil
	backend.cheatUnavailable = ""
	backend.cheatImported = false
	backend.cheatCatalogSource = ""
	backend.cheatIdentity = ""
	backend.cheatApplyWarning = ""
	backend.imageSHA256 = ""
	backend.mu.Unlock()

	var errs []error
	if machine != nil {
		// Flush the title's writable storage so saves survive a close/reopen.
		backend.persistSaveData(machine, closingHash)
		errs = append(errs, machine.Close())
	}
	if sourceFile != nil {
		errs = append(errs, sourceFile.Close())
	}
	return errors.Join(errs...)
}

func (backend *Backend) currentMachine() aramcore.Machine {
	backend.mu.RLock()
	defer backend.mu.RUnlock()
	return backend.machine
}

func (backend *Backend) runningRequested() bool {
	backend.mu.RLock()
	defer backend.mu.RUnlock()
	return backend.runRequested
}

func (backend *Backend) setRunRequested(requested bool) {
	backend.mu.Lock()
	backend.runRequested = requested
	backend.mu.Unlock()
}

func machineCanContinue(state aramcore.State) bool {
	return state == aramcore.StateReady ||
		state == aramcore.StateRunning ||
		state == aramcore.StatePaused
}

func (backend *Backend) saveState(slot int) error {
	machine := backend.currentMachine()
	if machine == nil {
		return errors.New("no machine is loaded")
	}
	path, err := backend.statePath(slot)
	if err != nil {
		return err
	}
	header := stateHeader{
		Schema:      stateSchemaVersion,
		InputSHA256: backend.input.SHA256,
		ProfileID:   backend.input.ProfileID,
		Backend:     backend.BackendName(),
	}
	headerData, err := json.Marshal(header)
	if err != nil {
		return err
	}
	if len(headerData) > maxStateHeaderSize {
		return errors.New("state header exceeds the safety limit")
	}

	temporary, err := os.CreateTemp(filepath.Dir(path), "state-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		if !committed {
			_ = temporary.Close()
			_ = os.Remove(temporaryPath)
		}
	}()
	if _, err := temporary.Write(stateMagic); err != nil {
		return err
	}
	if err := binary.Write(temporary, binary.LittleEndian, uint32(len(headerData))); err != nil {
		return err
	}
	if _, err := temporary.Write(headerData); err != nil {
		return err
	}
	if err := machine.SaveState(temporary); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := replaceFileCrashSafely(temporaryPath, path); err != nil {
		return err
	}
	committed = true
	return nil
}

func (backend *Backend) loadState(slot int) error {
	machine := backend.currentMachine()
	if machine == nil {
		return errors.New("no machine is loaded")
	}
	path, err := backend.statePath(slot)
	if err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	magic := make([]byte, len(stateMagic))
	if _, err := io.ReadFull(file, magic); err != nil {
		return fmt.Errorf("read state magic: %w", err)
	}
	if !bytes.Equal(magic, stateMagic) {
		return errors.New("state file has invalid magic")
	}
	var headerSize uint32
	if err := binary.Read(file, binary.LittleEndian, &headerSize); err != nil {
		return fmt.Errorf("read state header size: %w", err)
	}
	if headerSize == 0 || headerSize > maxStateHeaderSize {
		return errors.New("state header size is invalid")
	}
	headerData := make([]byte, headerSize)
	if _, err := io.ReadFull(file, headerData); err != nil {
		return fmt.Errorf("read state header: %w", err)
	}
	var header stateHeader
	if err := json.Unmarshal(headerData, &header); err != nil {
		return fmt.Errorf("decode state header: %w", err)
	}
	if err := backend.validateStateHeader(header); err != nil {
		return err
	}
	return machine.LoadState(file)
}

func (backend *Backend) statePath(slot int) (string, error) {
	if slot < 0 || slot > 9 {
		return "", fmt.Errorf("state slot %d is outside 0-9", slot)
	}
	backend.mu.RLock()
	root := backend.stateRoot
	hash := backend.input.SHA256
	backend.mu.RUnlock()
	if root == "" {
		configRoot, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(configRoot, "ARAM", "states")
	}
	if hash == "" {
		return "", errors.New("loaded input has no SHA-256 identity")
	}
	directory := filepath.Join(root, hash)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(directory, fmt.Sprintf("slot-%d.aramstate", slot)), nil
}

func (backend *Backend) validateStateHeader(header stateHeader) error {
	backend.mu.RLock()
	input := backend.input
	backend.mu.RUnlock()
	switch {
	case header.Schema != stateSchemaVersion:
		return fmt.Errorf("state schema %d is unsupported", header.Schema)
	case header.InputSHA256 != input.SHA256:
		return errors.New("state belongs to a different input hash")
	case header.ProfileID != input.ProfileID:
		return errors.New("state belongs to a different compatibility profile")
	case header.Backend != backend.BackendName():
		return errors.New("state belongs to a different backend")
	default:
		return nil
	}
}

type stateHeader struct {
	Schema      int    `json:"schema"`
	InputSHA256 string `json:"input_sha256"`
	ProfileID   string `json:"profile_id"`
	Backend     string `json:"backend"`
}

func replaceFileCrashSafely(temporaryPath, targetPath string) error {
	backupPath := targetPath + ".bak"
	_ = os.Remove(backupPath)
	if _, err := os.Stat(targetPath); err == nil {
		if err := os.Rename(targetPath, backupPath); err != nil {
			return err
		}
	}
	if err := os.Rename(temporaryPath, targetPath); err != nil {
		_ = os.Rename(backupPath, targetPath)
		return err
	}
	_ = os.Remove(backupPath)
	return nil
}

func frameFingerprint(frame image.Image) uint64 {
	hash := fnv.New64a()
	bounds := frame.Bounds()
	_ = binary.Write(hash, binary.LittleEndian, int64(bounds.Dx()))
	_ = binary.Write(hash, binary.LittleEndian, int64(bounds.Dy()))
	if rgba, ok := frame.(*image.RGBA); ok {
		rowBytes := bounds.Dx() * 4
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			offset := rgba.PixOffset(bounds.Min.X, y)
			_, _ = hash.Write(rgba.Pix[offset : offset+rowBytes])
		}
		return hash.Sum64()
	}
	var pixel [8]byte
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := frame.At(x, y).RGBA()
			binary.LittleEndian.PutUint16(pixel[0:2], uint16(r))
			binary.LittleEndian.PutUint16(pixel[2:4], uint16(g))
			binary.LittleEndian.PutUint16(pixel[4:6], uint16(b))
			binary.LittleEndian.PutUint16(pixel[6:8], uint16(a))
			_, _ = hash.Write(pixel[:])
		}
	}
	return hash.Sum64()
}

func backendError(kind frontend.FailureKind, err error) error {
	return &frontend.BackendError{
		Kind:    kind,
		Backend: "aram-core",
		Reason:  err.Error(),
		Err:     err,
	}
}

func classifyMachineError(machine aramcore.Machine, err error) frontend.FailureKind {
	if errors.Is(err, aramcore.ErrBackendUnavailable) {
		return frontend.FailureBackendUnavailable
	}
	if errors.Is(err, cpu.ErrUnsupportedInstruction) ||
		errors.Is(err, cpu.ErrInvalidAddress) ||
		errors.Is(err, cpu.ErrPermissionDenied) {
		return frontend.FailureGuestFaulted
	}
	if machine != nil && machine.State() == aramcore.StateFaulted {
		return frontend.FailureGuestFaulted
	}
	return frontend.FailureUnknown
}

func classifyFactoryError(err error, sourceFormat string) frontend.FailureKind {
	switch {
	case errors.Is(err, aramcore.ErrBackendUnavailable):
		return frontend.FailureBackendUnavailable
	case errors.Is(err, application.ErrUnsupportedSource):
		return frontend.FailureUnsupportedProfile
	case errors.Is(err, loader.ErrNoContainerRecords):
		if sourceFormat != string(loader.KindDAT) &&
			sourceFormat != string(loader.KindEADS) {
			return frontend.FailureUnsupportedProfile
		}
		return frontend.FailureMalformedInput
	default:
		return frontend.FailureMalformedInput
	}
}

func modeName(mode cpu.Mode) string {
	if mode == cpu.ModeThumb {
		return "Thumb"
	}
	return "ARM"
}

func displayName(request frontend.OpenRequest) string {
	if request.DisplayName != "" {
		return request.DisplayName
	}
	if request.Path != "" {
		return filepath.Base(request.Path)
	}
	return "document"
}

func emptyFallback(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
