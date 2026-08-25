//go:build system_firmware

// Package systemintegration adapts aram-core whole-phone machines to the
// shared aram-frontend backend contract. It lives in aram-emu so neither the
// headless core nor the reusable frontend needs to know about the other.
package systemintegration

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/mirusu400/aram-core/cpu"
	"github.com/mirusu400/aram-core/firmwareset"
	"github.com/mirusu400/aram-core/systemmachine"
	"github.com/mirusu400/aram-frontend/frontend"
)

const (
	DefaultInstructionsPerFrame     = uint64(1_000_000)
	DefaultMinimumInputInstructions = uint64(7_000_000)
	mediaSchemaVersion              = uint32(1)
	maxMediaBuildIDBytes            = uint32(4 * 1024)
	maxMediaSectionBytes            = uint64(2 << 30)
)

var mediaMagic = []byte("ARAMSYSMEDIA\x00")

type systemMachine interface {
	Identity() systemmachine.Identity
	Position() systemmachine.Position
	Controls() []string
	Run(context.Context, uint64) cpu.Result
	Stop() error
	SetKey(string, bool) error
	Framebuffer() image.Image
	FrameSHA256() string
	PowerCycle() error
	SaveMedia() (systemmachine.MediaState, error)
	LoadMedia(systemmachine.MediaState) error
	Close() error
}

type machineFactory func(firmwareset.Set, systemmachine.Options) (systemMachine, error)

// Options controls host integration policy rather than emulated hardware.
// MinimumInputInstructions prevents a normal host click from disappearing
// between the much slower guest keypad scans. MediaRoot is primarily useful
// to portable packages and tests; an empty root uses the operating system's
// per-user configuration directory.
type Options struct {
	InstructionsPerFrame     uint64
	MinimumInputInstructions uint64
	MediaRoot                string
	DisableMediaPersistence  bool

	newMachine machineFactory
}

// Backend presents a synchronous whole-phone machine through the frontend's
// asynchronous frame scheduler. All guest execution and lifecycle operations
// are serialized without blocking frame reads on the UI thread.
type Backend struct {
	operationMu sync.Mutex
	mu          sync.RWMutex

	options       Options
	machine       systemMachine
	firmwareFiles []*os.File
	input         frontend.InputInfo
	state         frontend.BackendState
	contentID     string
	mediaWarning  string
	controls      map[string]bool
	frameHash     string
	frameSequence uint64
	inputSources  map[string]string
	inputHolds    map[string]inputHold
}

type inputHold struct {
	sources        int
	releaseAfter   uint64
	releasePending bool
}

func NewBackend(options Options) *Backend {
	if options.InstructionsPerFrame == 0 {
		options.InstructionsPerFrame = DefaultInstructionsPerFrame
	}
	if options.MinimumInputInstructions == 0 {
		options.MinimumInputInstructions = DefaultMinimumInputInstructions
	}
	if options.newMachine == nil {
		options.newMachine = func(set firmwareset.Set, options systemmachine.Options) (systemMachine, error) {
			return systemmachine.New(set, options)
		}
	}
	return &Backend{
		options:      options,
		state:        frontend.StateEmpty,
		inputSources: make(map[string]string),
		inputHolds:   make(map[string]inputHold),
	}
}

func (backend *Backend) BackendName() string { return "aram-core system machine" }

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
	set, files, info, contentID, err := inspectFirmwareDirectory(request)
	if err != nil {
		return info, systemBackendError(frontend.FailureMalformedInput, err)
	}
	closeFiles := func() {
		for _, file := range files {
			_ = file.Close()
		}
	}
	if progress != nil {
		progress(frontend.OpenStageLoading)
	}
	machine, err := backend.options.newMachine(set, systemmachine.Options{})
	if err != nil {
		closeFiles()
		kind := frontend.FailureUnsupportedProfile
		if errors.Is(err, systemmachine.ErrUnsupportedBackend) {
			kind = frontend.FailureBackendUnavailable
		}
		return info, systemBackendError(kind, err)
	}
	identity := machine.Identity()
	info.Format = "Samsung system firmware set"
	info.ProfileID = identity.FirmwareBuildID

	mediaWarning := ""
	if !backend.options.DisableMediaPersistence {
		if restoreErr := backend.restoreMedia(machine, contentID); restoreErr != nil {
			// A stale or interrupted local media file must not make the immutable
			// firmware unbootable. Keep the fresh machine and report the warning
			// through the compatibility panel.
			mediaWarning = restoreErr.Error()
		}
	}
	controls := make(map[string]bool)
	for _, control := range machine.Controls() {
		controls[control] = true
	}

	backend.mu.Lock()
	oldMachine := backend.machine
	oldFiles := backend.firmwareFiles
	backend.machine = machine
	backend.firmwareFiles = files
	backend.input = info
	backend.state = frontend.StateReady
	backend.contentID = contentID
	backend.mediaWarning = mediaWarning
	backend.controls = controls
	backend.frameHash = ""
	backend.frameSequence = 0
	backend.resetInputState()
	backend.mu.Unlock()

	if oldMachine != nil {
		_ = oldMachine.Close()
	}
	for _, file := range oldFiles {
		_ = file.Close()
	}
	return info, nil
}

func inspectFirmwareDirectory(
	request frontend.OpenRequest,
) (firmwareset.Set, []*os.File, frontend.InputInfo, string, error) {
	name := request.DisplayName
	if name == "" && request.Path != "" {
		name = filepath.Base(filepath.Clean(request.Path))
	}
	info := frontend.InputInfo{DisplayName: name, Format: "firmware-directory"}
	if request.Path == "" {
		return firmwareset.Set{}, nil, info, "", errors.New("no firmware directory was selected")
	}
	directory, err := os.Stat(request.Path)
	if err != nil {
		return firmwareset.Set{}, nil, info, "", err
	}
	if !directory.IsDir() {
		return firmwareset.Set{}, nil, info, "", errors.New("system firmware input must be an extracted directory")
	}
	entries, err := os.ReadDir(request.Path)
	if err != nil {
		return firmwareset.Set{}, nil, info, "", err
	}
	var files []*os.File
	var sources []firmwareset.Source
	closeFiles := func() {
		for _, file := range files {
			_ = file.Close()
		}
	}
	for _, entry := range entries {
		if entry.IsDir() || !firmwarePieceExtension(filepath.Ext(entry.Name())) {
			continue
		}
		file, openErr := os.Open(filepath.Join(request.Path, entry.Name()))
		if openErr != nil {
			closeFiles()
			return firmwareset.Set{}, nil, info, "", openErr
		}
		fileInfo, statErr := file.Stat()
		if statErr != nil {
			_ = file.Close()
			closeFiles()
			return firmwareset.Set{}, nil, info, "", statErr
		}
		files = append(files, file)
		sources = append(sources, firmwareset.Source{ReaderAt: file, Size: fileInfo.Size()})
	}
	if len(sources) == 0 {
		closeFiles()
		return firmwareset.Set{}, nil, info, "", errors.New(
			"directory has no Samsung .wbt, .wbin, .dat, or .fnt firmware pieces",
		)
	}
	set, err := firmwareset.NewSet(sources)
	if err != nil {
		closeFiles()
		return firmwareset.Set{}, nil, info, "", err
	}
	contentID := firmwareContentID(set)
	info.Size = set.TotalSize()
	info.SHA256 = contentID
	return set, files, info, contentID, nil
}

func firmwarePieceExtension(extension string) bool {
	switch strings.ToLower(extension) {
	case ".wbt", ".wbin", ".dat", ".fnt":
		return true
	default:
		return false
	}
}

// firmwareContentID is independent of host paths and piece names. Sorting the
// verified piece facts also makes renaming files leave persistence identity
// unchanged.
func firmwareContentID(set firmwareset.Set) string {
	pieces := append([]firmwareset.PieceManifest(nil), set.Manifest().Pieces...)
	sort.Slice(pieces, func(left, right int) bool {
		if pieces[left].SHA256 != pieces[right].SHA256 {
			return pieces[left].SHA256 < pieces[right].SHA256
		}
		return pieces[left].Size < pieces[right].Size
	})
	hash := sha256.New()
	_, _ = io.WriteString(hash, firmwareset.ManifestSchema)
	var scalar [8]byte
	binary.LittleEndian.PutUint64(scalar[:], uint64(len(pieces)))
	_, _ = hash.Write(scalar[:])
	for _, piece := range pieces {
		binary.LittleEndian.PutUint64(scalar[:], uint64(piece.Size))
		_, _ = hash.Write(scalar[:])
		_, _ = io.WriteString(hash, piece.SHA256)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func (backend *Backend) State() frontend.BackendState {
	backend.mu.RLock()
	defer backend.mu.RUnlock()
	return backend.state
}

func (backend *Backend) Supports(command frontend.BackendCommand) bool {
	return backend.Capability(command).Supported
}

func (backend *Backend) Capability(command frontend.BackendCommand) frontend.Capability {
	state := backend.State()
	supported := false
	switch command {
	case frontend.CommandStart:
		supported = state == frontend.StateReady || state == frontend.StatePaused || state == frontend.StateStopped
	case frontend.CommandPauseResume:
		supported = state == frontend.StateRunning || state == frontend.StatePaused
	case frontend.CommandStop:
		supported = state == frontend.StateRunning || state == frontend.StatePaused
	case frontend.CommandReset:
		supported = state != frontend.StateEmpty && state != frontend.StateFaulted
	case frontend.CommandFrame:
		supported = state == frontend.StateReady || state == frontend.StatePaused
	case frontend.CommandFastForward:
		return frontend.Capability{Reason: "Whole-phone fast-forward is not calibrated yet"}
	case frontend.CommandLoadState, frontend.CommandSaveState:
		return frontend.Capability{Reason: "Whole-phone save states are not exposed by this adapter yet"}
	case frontend.CommandRewind:
		return frontend.Capability{Reason: "Whole-phone rewind history is not implemented yet"}
	default:
		return frontend.Capability{Reason: "Unknown backend command"}
	}
	if !supported {
		return frontend.Capability{Reason: fmt.Sprintf("%s is unavailable while the machine is %s", command, state)}
	}
	return frontend.Capability{Supported: true}
}

func (backend *Backend) Execute(ctx context.Context, command frontend.BackendCommand) error {
	return backend.ExecuteCommand(ctx, frontend.CommandRequest{Command: command, Speed: 1})
}

func (backend *Backend) ExecuteCommand(
	_ context.Context,
	request frontend.CommandRequest,
) error {
	backend.operationMu.Lock()
	defer backend.operationMu.Unlock()
	if capability := backend.Capability(request.Command); !capability.Supported {
		return errors.New(capability.Reason)
	}
	machine := backend.currentMachine()
	if machine == nil {
		return systemBackendError(frontend.FailureBackendUnavailable, errors.New("no system machine is loaded"))
	}

	var err error
	switch request.Command {
	case frontend.CommandStart:
		backend.setState(frontend.StateRunning)
	case frontend.CommandPauseResume:
		if backend.State() == frontend.StateRunning {
			backend.setState(frontend.StatePaused)
		} else {
			backend.setState(frontend.StateRunning)
		}
	case frontend.CommandStop:
		err = machine.Stop()
		if err == nil {
			backend.setState(frontend.StateStopped)
			err = backend.persistCurrentMedia(machine)
		}
	case frontend.CommandReset:
		err = machine.PowerCycle()
		if err == nil {
			backend.resetInputState()
			backend.setState(frontend.StateReady)
			backend.resetFrameIdentity()
		}
	case frontend.CommandFrame:
		err = backend.runMachineFrame(machine)
	}
	if err != nil {
		backend.setState(frontend.StateFaulted)
		return systemBackendError(frontend.FailureGuestFaulted, err)
	}
	return nil
}

func (backend *Backend) RunFrame(ctx context.Context) error {
	backend.operationMu.Lock()
	defer backend.operationMu.Unlock()
	if backend.State() != frontend.StateRunning {
		return nil
	}
	machine := backend.currentMachine()
	if machine == nil {
		backend.setState(frontend.StateEmpty)
		return systemBackendError(frontend.FailureBackendUnavailable, errors.New("no system machine is loaded"))
	}
	if err := backend.runMachineFrameContext(ctx, machine); err != nil {
		backend.setState(frontend.StateFaulted)
		return systemBackendError(frontend.FailureGuestFaulted, err)
	}
	return nil
}

func (backend *Backend) runMachineFrame(machine systemMachine) error {
	return backend.runMachineFrameContext(context.Background(), machine)
}

func (backend *Backend) runMachineFrameContext(ctx context.Context, machine systemMachine) error {
	result := machine.Run(ctx, backend.options.InstructionsPerFrame)
	if result.Err != nil {
		return result.Err
	}
	if err := backend.flushInputReleases(machine); err != nil {
		return err
	}
	switch result.Reason {
	case cpu.StopBudget:
		return nil
	case cpu.StopRequested, cpu.StopExited:
		backend.setState(frontend.StateStopped)
		return nil
	case cpu.StopBreakpoint, cpu.StopExecutionTrap:
		backend.setState(frontend.StatePaused)
		return nil
	default:
		return fmt.Errorf("whole-phone CPU stopped unexpectedly at PC 0x%08x (reason %d)", result.PC, result.Reason)
	}
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
	hash := machine.FrameSHA256()
	backend.mu.Lock()
	if backend.frameSequence == 0 || hash != backend.frameHash {
		backend.frameSequence++
		backend.frameHash = hash
	}
	sequence := backend.frameSequence
	backend.mu.Unlock()
	return frontend.VideoFrame{Image: frame, Sequence: sequence}
}

func (backend *Backend) QueueInput(event frontend.InputEvent) error {
	backend.operationMu.Lock()
	defer backend.operationMu.Unlock()

	machine := backend.currentMachine()
	if machine == nil {
		return systemBackendError(frontend.FailureBackendUnavailable, errors.New("no system machine is loaded"))
	}
	control := systemControl(event.Control)
	backend.mu.RLock()
	supported := backend.controls[control]
	// Feature-phone menus are normally reached through the left soft button,
	// while the shared frontend also exposes an abstract MENU control for
	// application runtimes. Use the physical soft button only when a board has
	// no distinct menu key of its own.
	if !supported && control == "menu" && backend.controls["soft-left"] {
		control = "soft-left"
		supported = true
	}
	backend.mu.RUnlock()
	if !supported {
		return fmt.Errorf("firmware board profile does not expose control %q yet", event.Control)
	}
	if event.Pressed {
		if _, alreadyPressed := backend.inputSources[event.Control]; alreadyPressed {
			return nil
		}
		hold := backend.inputHolds[control]
		if hold.sources == 0 {
			if err := machine.SetKey(control, true); err != nil {
				return systemBackendError(frontend.FailureUnknown, err)
			}
		}
		hold.sources++
		hold.releasePending = false
		releaseAfter := addSaturating(
			machine.Position().Instructions,
			backend.options.MinimumInputInstructions,
		)
		if releaseAfter > hold.releaseAfter {
			hold.releaseAfter = releaseAfter
		}
		backend.inputSources[event.Control] = control
		backend.inputHolds[control] = hold
		return nil
	}

	pressedControl, wasPressed := backend.inputSources[event.Control]
	if !wasPressed {
		return nil
	}
	delete(backend.inputSources, event.Control)
	hold := backend.inputHolds[pressedControl]
	if hold.sources > 0 {
		hold.sources--
	}
	if hold.sources > 0 {
		backend.inputHolds[pressedControl] = hold
		return nil
	}
	if machine.Position().Instructions < hold.releaseAfter {
		hold.releasePending = true
		backend.inputHolds[pressedControl] = hold
		return nil
	}
	if err := machine.SetKey(pressedControl, false); err != nil {
		return systemBackendError(frontend.FailureUnknown, err)
	}
	delete(backend.inputHolds, pressedControl)
	return nil
}

func (backend *Backend) flushInputReleases(machine systemMachine) error {
	position := machine.Position().Instructions
	for control, hold := range backend.inputHolds {
		if hold.sources != 0 || !hold.releasePending || position < hold.releaseAfter {
			continue
		}
		if err := machine.SetKey(control, false); err != nil {
			return systemBackendError(frontend.FailureUnknown, err)
		}
		delete(backend.inputHolds, control)
	}
	return nil
}

func (backend *Backend) resetInputState() {
	clear(backend.inputSources)
	clear(backend.inputHolds)
}

func addSaturating(left, right uint64) uint64 {
	if ^uint64(0)-left < right {
		return ^uint64(0)
	}
	return left + right
}

func systemControl(control string) string {
	if strings.HasPrefix(control, "num") && len(control) == len("num0") && control[3] >= '0' && control[3] <= '9' {
		return "digit-" + control[3:]
	}
	if control == "hash" {
		return "pound"
	}
	return control
}

func (backend *Backend) ToolSnapshot(
	_ context.Context,
	kind frontend.ToolKind,
) (frontend.ToolSnapshot, error) {
	if kind != frontend.ToolCompatibility && kind != frontend.ToolDebugger && kind != frontend.ToolLogs {
		return frontend.ToolSnapshot{}, fmt.Errorf("whole-phone %s tools are not implemented yet", kind)
	}
	backend.mu.RLock()
	machine := backend.machine
	input := backend.input
	warning := backend.mediaWarning
	backend.mu.RUnlock()
	if machine == nil {
		return frontend.ToolSnapshot{}, errors.New("no system machine is loaded")
	}
	identity := machine.Identity()
	position := machine.Position()
	lines := []string{
		"Input: " + input.DisplayName,
		"Firmware set SHA-256: " + input.SHA256,
		"Model: " + identity.Manufacturer + " " + identity.Model,
		"Build: " + identity.FirmwareBuild,
		"Profile: " + identity.FirmwareBuildID,
		"Board: " + identity.BoardID,
		"Platform: " + identity.PlatformID,
		fmt.Sprintf("CPU: %s | PC 0x%08x | instructions %d", identity.CPU.Name, position.PC, position.Instructions),
	}
	if warning != "" {
		lines = append(lines, "Persistent media warning: "+warning)
	}
	return frontend.ToolSnapshot{Title: "Whole-phone Compatibility", Lines: lines}, nil
}

func (backend *Backend) Close() error {
	backend.operationMu.Lock()
	defer backend.operationMu.Unlock()

	backend.mu.RLock()
	machine := backend.machine
	backend.mu.RUnlock()
	var errs []error
	if machine != nil {
		// Persist while contentID is still available. Clearing the backend first
		// silently skipped media saving on ordinary window close.
		errs = append(errs, backend.persistCurrentMedia(machine))
	}

	backend.mu.Lock()
	files := backend.firmwareFiles
	backend.machine = nil
	backend.firmwareFiles = nil
	backend.input = frontend.InputInfo{}
	backend.state = frontend.StateEmpty
	backend.contentID = ""
	backend.mediaWarning = ""
	backend.controls = nil
	backend.frameHash = ""
	backend.frameSequence = 0
	backend.resetInputState()
	backend.mu.Unlock()

	if machine != nil {
		errs = append(errs, machine.Close())
	}
	for _, file := range files {
		errs = append(errs, file.Close())
	}
	return errors.Join(errs...)
}

func (backend *Backend) currentMachine() systemMachine {
	backend.mu.RLock()
	defer backend.mu.RUnlock()
	return backend.machine
}

func (backend *Backend) setState(state frontend.BackendState) {
	backend.mu.Lock()
	backend.state = state
	backend.mu.Unlock()
}

func (backend *Backend) resetFrameIdentity() {
	backend.mu.Lock()
	backend.frameHash = ""
	backend.frameSequence = 0
	backend.mu.Unlock()
}

func (backend *Backend) persistCurrentMedia(machine systemMachine) error {
	if backend.options.DisableMediaPersistence || machine == nil {
		return nil
	}
	backend.mu.RLock()
	contentID := backend.contentID
	backend.mu.RUnlock()
	if contentID == "" {
		return nil
	}
	media, err := machine.SaveMedia()
	if err != nil {
		return fmt.Errorf("capture persistent phone media: %w", err)
	}
	path, err := backend.mediaPath(contentID)
	if err != nil {
		return err
	}
	if err := writeMediaFile(path, media); err != nil {
		return fmt.Errorf("save persistent phone media: %w", err)
	}
	return nil
}

func (backend *Backend) restoreMedia(machine systemMachine, contentID string) error {
	path, err := backend.mediaPath(contentID)
	if err != nil {
		return err
	}
	media, err := readMediaFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read persistent phone media: %w", err)
	}
	if err := machine.LoadMedia(media); err != nil {
		return fmt.Errorf("restore persistent phone media: %w", err)
	}
	return nil
}

func (backend *Backend) mediaPath(contentID string) (string, error) {
	root := backend.options.MediaRoot
	if root == "" {
		configRoot, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(configRoot, "ARAM", "system-media")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(root, contentID+".arammedia"), nil
}

func writeMediaFile(path string, media systemmachine.MediaState) error {
	if media.FirmwareBuildID == "" || len(media.FirmwareBuildID) > int(maxMediaBuildIDBytes) {
		return errors.New("persistent media has an invalid firmware build ID")
	}
	if uint64(len(media.Flash)) > maxMediaSectionBytes || uint64(len(media.NAND)) > maxMediaSectionBytes {
		return errors.New("persistent media exceeds the host integration limit")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".arammedia-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	fail := func(writeErr error) error {
		_ = temporary.Close()
		return writeErr
	}
	if _, err := temporary.Write(mediaMagic); err != nil {
		return fail(err)
	}
	hash := sha256.New()
	output := io.MultiWriter(temporary, hash)
	for _, value := range []any{
		mediaSchemaVersion,
		uint32(len(media.FirmwareBuildID)),
		uint64(len(media.Flash)),
		uint64(len(media.NAND)),
	} {
		if err := binary.Write(output, binary.LittleEndian, value); err != nil {
			return fail(err)
		}
	}
	for _, bytes := range [][]byte{[]byte(media.FirmwareBuildID), media.Flash, media.NAND} {
		if _, err := output.Write(bytes); err != nil {
			return fail(err)
		}
	}
	if _, err := temporary.Write(hash.Sum(nil)); err != nil {
		return fail(err)
	}
	if err := temporary.Sync(); err != nil {
		return fail(err)
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return replaceFile(path, temporaryPath)
}

func readMediaFile(path string) (systemmachine.MediaState, error) {
	file, err := os.Open(path)
	if err != nil {
		return systemmachine.MediaState{}, err
	}
	defer file.Close()
	magic := make([]byte, len(mediaMagic))
	if _, err := io.ReadFull(file, magic); err != nil {
		return systemmachine.MediaState{}, err
	}
	if string(magic) != string(mediaMagic) {
		return systemmachine.MediaState{}, errors.New("persistent media magic is invalid")
	}
	hash := sha256.New()
	input := io.TeeReader(file, hash)
	var version, buildLength uint32
	var flashLength, nandLength uint64
	for _, value := range []any{&version, &buildLength, &flashLength, &nandLength} {
		if err := binary.Read(input, binary.LittleEndian, value); err != nil {
			return systemmachine.MediaState{}, err
		}
	}
	if version != mediaSchemaVersion {
		return systemmachine.MediaState{}, fmt.Errorf("persistent media schema %d is unsupported", version)
	}
	if buildLength == 0 || buildLength > maxMediaBuildIDBytes ||
		flashLength == 0 || flashLength > maxMediaSectionBytes ||
		nandLength == 0 || nandLength > maxMediaSectionBytes {
		return systemmachine.MediaState{}, errors.New("persistent media section lengths are invalid")
	}
	build := make([]byte, int(buildLength))
	flash := make([]byte, int(flashLength))
	nand := make([]byte, int(nandLength))
	for _, bytes := range [][]byte{build, flash, nand} {
		if _, err := io.ReadFull(input, bytes); err != nil {
			return systemmachine.MediaState{}, err
		}
	}
	wantDigest := make([]byte, sha256.Size)
	if _, err := io.ReadFull(file, wantDigest); err != nil {
		return systemmachine.MediaState{}, err
	}
	var trailing [1]byte
	if count, trailingErr := file.Read(trailing[:]); count != 0 || trailingErr == nil {
		return systemmachine.MediaState{}, errors.New("persistent media has trailing data")
	} else if !errors.Is(trailingErr, io.EOF) {
		return systemmachine.MediaState{}, trailingErr
	}
	if !equalBytes(hash.Sum(nil), wantDigest) {
		return systemmachine.MediaState{}, errors.New("persistent media checksum does not match")
	}
	return systemmachine.MediaState{
		FirmwareBuildID: string(build),
		Flash:           flash,
		NAND:            nand,
	}, nil
}

func replaceFile(path, temporaryPath string) error {
	backupPath := path + ".bak"
	_ = os.Remove(backupPath)
	if _, err := os.Stat(path); err == nil {
		if err := os.Rename(path, backupPath); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		_ = os.Rename(backupPath, path)
		return err
	}
	_ = os.Remove(backupPath)
	return nil
}

func equalBytes(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	var difference byte
	for index := range left {
		difference |= left[index] ^ right[index]
	}
	return difference == 0
}

func systemBackendError(kind frontend.FailureKind, err error) error {
	return &frontend.BackendError{
		Kind:    kind,
		Backend: "aram-core system machine",
		Reason:  err.Error(),
		Err:     err,
	}
}

var (
	_ frontend.OpenProgressBackend = (*Backend)(nil)
	_ frontend.CapabilityBackend   = (*Backend)(nil)
	_ frontend.CommandBackend      = (*Backend)(nil)
	_ frontend.VideoBackend        = (*Backend)(nil)
	_ frontend.FrameBackend        = (*Backend)(nil)
	_ frontend.InputBackend        = (*Backend)(nil)
	_ frontend.ToolBackend         = (*Backend)(nil)
	_ frontend.BackendNamer        = (*Backend)(nil)
)
