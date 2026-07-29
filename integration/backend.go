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
	aramcore "github.com/mirusu400/aram-core/core"
	"github.com/mirusu400/aram-core/cpu"
	"github.com/mirusu400/aram-core/loader"
	"github.com/mirusu400/aram-frontend/frontend"
)

const (
	stateSchemaVersion = 1
	maxStateHeaderSize = 64 * 1024
)

var stateMagic = []byte("ARAMSTATE\x00")

type Backend struct {
	mu            sync.RWMutex
	factory       aramcore.Factory
	machine       aramcore.Machine
	sourceFile    *os.File
	source        aramcore.Source
	input         frontend.InputInfo
	stateRoot     string
	audio         frontend.AudioSettings
	lastFrameHash uint64
	frameSequence uint64
}

func NewBackend(factory aramcore.Factory) *Backend {
	if factory == nil {
		defaultFactory := application.NewFactory()
		factory = defaultFactory
	}
	return &Backend{factory: factory}
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
	machine, err := backend.factory.Create(ctx, source)
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
	if provider, ok := machine.(interface {
		ImageInfo() application.ImageInfo
	}); ok {
		imageInfo := provider.ImageInfo()
		info.Format = string(imageInfo.SourceKind)
		info.ProfileID = imageInfo.ProfileID
		source.ProfileID = imageInfo.ProfileID
	}

	backend.mu.Lock()
	oldMachine := backend.machine
	oldFile := backend.sourceFile
	backend.machine = machine
	backend.sourceFile = sourceFile
	backend.source = source
	backend.input = info
	backend.lastFrameHash = 0
	backend.frameSequence = 0
	backend.mu.Unlock()

	if oldMachine != nil {
		_ = oldMachine.Close()
	}
	if oldFile != nil {
		_ = oldFile.Close()
	}
	return info, nil
}

func inspectSource(
	request frontend.OpenRequest,
) (aramcore.Source, frontend.InputInfo, *os.File, error) {
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

func (backend *Backend) State() frontend.BackendState {
	machine := backend.currentMachine()
	if machine == nil {
		return frontend.StateEmpty
	}
	switch machine.State() {
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
	state := machine.State()
	supported := false
	switch command {
	case frontend.CommandStart:
		supported = state == aramcore.StateReady || state == aramcore.StatePaused
	case frontend.CommandPauseResume:
		supported = state == aramcore.StateRunning || state == aramcore.StatePaused
	case frontend.CommandStop:
		supported = state == aramcore.StateRunning || state == aramcore.StatePaused
	case frontend.CommandReset:
		supported = state != aramcore.StateEmpty && state != aramcore.StateFaulted
	case frontend.CommandFrame:
		supported = state == aramcore.StateReady ||
			state == aramcore.StatePaused
	case frontend.CommandLoadState, frontend.CommandSaveState:
		supported = state != aramcore.StateEmpty && state != aramcore.StateRunning
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
		err = machine.Start(ctx)
	case frontend.CommandPauseResume:
		if machine.State() == aramcore.StatePaused {
			err = machine.Resume()
		} else {
			err = machine.Pause()
		}
	case frontend.CommandStop:
		err = machine.Stop()
	case frontend.CommandReset:
		err = machine.Reset(ctx)
	case frontend.CommandFrame:
		err = machine.StepFrame(ctx)
	case frontend.CommandSaveState:
		err = backend.saveState(request.Slot)
	case frontend.CommandLoadState:
		err = backend.loadState(request.Slot)
	default:
		err = fmt.Errorf("unsupported backend command %q", request.Command)
	}
	if err != nil {
		return backendError(classifyMachineError(machine, err), err)
	}
	return nil
}

func (backend *Backend) VideoFrame() frontend.VideoFrame {
	machine := backend.currentMachine()
	if machine == nil {
		return frontend.VideoFrame{}
	}
	frame := machine.Framebuffer()
	if frame == nil || frame.Bounds().Dx() <= 0 || frame.Bounds().Dy() <= 0 {
		return frontend.VideoFrame{}
	}
	fingerprint := frameFingerprint(frame)
	backend.mu.Lock()
	if backend.frameSequence == 0 || fingerprint != backend.lastFrameHash {
		backend.frameSequence++
		backend.lastFrameHash = fingerprint
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
	return nil
}

func (backend *Backend) ToolSnapshot(
	_ context.Context,
	kind frontend.ToolKind,
) (frontend.ToolSnapshot, error) {
	switch kind {
	case frontend.ToolCompatibility:
		backend.mu.RLock()
		input := backend.input
		source := backend.source
		backend.mu.RUnlock()
		return frontend.ToolSnapshot{
			Title: "Compatibility Report",
			Lines: []string{
				"Input: " + input.DisplayName,
				"Format: " + input.Format,
				"SHA-256: " + input.SHA256,
				"Profile: " + emptyFallback(input.ProfileID, "unselected"),
				"Core source: " + source.Path,
			},
		}, nil
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
		if provider, ok := machine.(interface {
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

func (backend *Backend) Close() error {
	backend.mu.Lock()
	machine := backend.machine
	sourceFile := backend.sourceFile
	backend.machine = nil
	backend.sourceFile = nil
	backend.source = aramcore.Source{}
	backend.input = frontend.InputInfo{}
	backend.lastFrameHash = 0
	backend.frameSequence = 0
	backend.mu.Unlock()

	var errs []error
	if machine != nil {
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
