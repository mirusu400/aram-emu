package systemintegration

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"github.com/mirusu400/aram-core/application"
	"github.com/mirusu400/aram-core/cpu"
	"github.com/mirusu400/aram-core/firmwareset"
	"github.com/mirusu400/aram-core/systemmachine"
	"github.com/mirusu400/aram-frontend/frontend"
)

type fakeSystemMachine struct {
	position    systemmachine.Position
	frame       *image.RGBA
	frameHash   string
	lastControl string
	lastPressed bool
	media       systemmachine.MediaState
	closed      bool
}

func newFakeSystemMachine() *fakeSystemMachine {
	frame := image.NewRGBA(image.Rect(0, 0, 240, 320))
	return &fakeSystemMachine{
		frame: frame,
		media: systemmachine.MediaState{
			FirmwareBuildID: "samsung.sch-w830.dl21",
			Flash:           []byte("flash-state"),
			NAND:            []byte("nand-state"),
		},
	}
}

func (machine *fakeSystemMachine) Identity() systemmachine.Identity {
	return systemmachine.Identity{
		Manufacturer:    "Samsung",
		Model:           "SCH-W830",
		FirmwareBuild:   "DL21",
		FirmwareBuildID: "samsung.sch-w830.dl21",
		BoardID:         "samsung-sch-w830",
		PlatformID:      "qualcomm-msm",
		CPU:             cpu.Identity{Name: "fake", Version: "1", Architecture: cpu.ARMv5TE},
	}
}

func (machine *fakeSystemMachine) Position() systemmachine.Position { return machine.position }
func (machine *fakeSystemMachine) Controls() []string {
	return []string{
		"soft-left", "soft-right", "up", "down", "left", "right", "ok", "back",
		"send", "end", "volume-up", "volume-down", "digit-0", "digit-1", "pound",
	}
}
func (machine *fakeSystemMachine) Run(_ context.Context, budget uint64) cpu.Result {
	machine.position.Instructions += budget
	machine.position.PC += 4
	shade := uint8(machine.position.Instructions)
	machine.frame.SetRGBA(0, 0, color.RGBA{R: shade, A: 0xff})
	machine.frameHash = string([]byte{shade})
	return cpu.Result{Reason: cpu.StopBudget, Instructions: budget, PC: machine.position.PC}
}
func (machine *fakeSystemMachine) Stop() error { return nil }
func (machine *fakeSystemMachine) SetKey(control string, pressed bool) error {
	machine.lastControl = control
	machine.lastPressed = pressed
	return nil
}
func (machine *fakeSystemMachine) Framebuffer() image.Image { return machine.frame }
func (machine *fakeSystemMachine) FrameSHA256() string      { return machine.frameHash }
func (machine *fakeSystemMachine) PowerCycle() error {
	machine.position = systemmachine.Position{}
	return nil
}
func (machine *fakeSystemMachine) SaveMedia() (systemmachine.MediaState, error) {
	return machine.media, nil
}
func (machine *fakeSystemMachine) LoadMedia(media systemmachine.MediaState) error {
	machine.media = media
	return nil
}
func (machine *fakeSystemMachine) Close() error {
	machine.closed = true
	return nil
}

func TestBackendOpensFirmwareDirectoryRunsFramesAndMapsControls(t *testing.T) {
	directory := t.TempDir()
	for name, data := range map[string]string{
		"phone.wbt":   "boot",
		"phone.wbin":  "code",
		"phone.dat":   "data",
		"phone.fnt":   "font",
		"ignored.txt": "ignored",
	} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	machine := newFakeSystemMachine()
	var selectedCPU cpu.Backend
	backend := NewBackend(Options{
		InstructionsPerFrame:     25,
		MinimumInputInstructions: 50,
		MediaRoot:                t.TempDir(),
		DisableMediaPersistence:  true,
		newMachine: func(set firmwareset.Set, options systemmachine.Options) (systemMachine, error) {
			if set.Len() != 4 {
				t.Fatalf("firmware pieces = %d, want 4", set.Len())
			}
			if options.Backend == nil || options.BackendMode != "" {
				t.Fatalf("default machine CPU options = %+v, want explicit fastest backend", options)
			}
			selectedCPU = options.Backend
			return machine, nil
		},
	})
	t.Cleanup(func() {
		_ = backend.Close()
		if selectedCPU != nil {
			_ = selectedCPU.Close()
		}
	})

	info, err := backend.Open(context.Background(), frontend.OpenRequest{Path: directory})
	if err != nil {
		t.Fatal(err)
	}
	if info.ProfileID != "samsung.sch-w830.dl21" || info.Size != 16 || len(info.SHA256) != 64 {
		t.Fatalf("input info = %+v", info)
	}
	if backend.State() != frontend.StateReady {
		t.Fatalf("state after open = %s", backend.State())
	}
	if err := backend.Execute(context.Background(), frontend.CommandStart); err != nil {
		t.Fatal(err)
	}
	if err := backend.RunFrame(context.Background()); err != nil {
		t.Fatal(err)
	}
	if machine.position.Instructions != 25 || backend.State() != frontend.StateRunning {
		t.Fatalf("position/state after frame = %+v/%s", machine.position, backend.State())
	}
	first := backend.VideoFrame()
	if first.Image == nil || first.Sequence != 1 {
		t.Fatalf("first video frame = %+v", first)
	}
	if second := backend.VideoFrame(); second.Sequence != first.Sequence {
		t.Fatalf("unchanged sequence = %d, want %d", second.Sequence, first.Sequence)
	}
	if err := backend.QueueInput(frontend.InputEvent{Control: "num1", Pressed: true}); err != nil {
		t.Fatal(err)
	}
	if machine.lastControl != "digit-1" || !machine.lastPressed {
		t.Fatalf("mapped input = %q/%t", machine.lastControl, machine.lastPressed)
	}
	if err := backend.QueueInput(frontend.InputEvent{Control: "num1", Pressed: false}); err != nil {
		t.Fatal(err)
	}
	if !machine.lastPressed {
		t.Fatal("short input pulse was released before its minimum guest duration")
	}
	if err := backend.RunFrame(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !machine.lastPressed {
		t.Fatal("input was released one frame before its minimum guest duration")
	}
	if err := backend.RunFrame(context.Background()); err != nil {
		t.Fatal(err)
	}
	if machine.lastControl != "digit-1" || machine.lastPressed {
		t.Fatalf("deferred input release = %q/%t", machine.lastControl, machine.lastPressed)
	}
	if err := backend.QueueInput(frontend.InputEvent{Control: "hash", Pressed: true}); err != nil {
		t.Fatal(err)
	}
	if machine.lastControl != "pound" {
		t.Fatalf("hash mapped to %q", machine.lastControl)
	}
	if err := backend.QueueInput(frontend.InputEvent{Control: "up", Pressed: true}); err != nil {
		t.Fatal(err)
	}
	if machine.lastControl != "up" {
		t.Fatalf("up mapped to %q", machine.lastControl)
	}
	if err := backend.QueueInput(frontend.InputEvent{Control: "volume-up", Pressed: true}); err != nil {
		t.Fatal(err)
	}
	if machine.lastControl != "volume-up" {
		t.Fatalf("volume-up mapped to %q", machine.lastControl)
	}
	if err := backend.QueueInput(frontend.InputEvent{Control: "volume-down", Pressed: true}); err != nil {
		t.Fatal(err)
	}
	if machine.lastControl != "volume-down" {
		t.Fatalf("volume-down mapped to %q", machine.lastControl)
	}
	if err := backend.QueueInput(frontend.InputEvent{Control: "back", Pressed: true}); err != nil {
		t.Fatal(err)
	}
	if machine.lastControl != "back" {
		t.Fatalf("back mapped to %q", machine.lastControl)
	}
	if err := backend.QueueInput(frontend.InputEvent{Control: "send", Pressed: true}); err != nil {
		t.Fatal(err)
	}
	if machine.lastControl != "send" {
		t.Fatalf("send mapped to %q", machine.lastControl)
	}
	if err := backend.QueueInput(frontend.InputEvent{Control: "end", Pressed: true}); err != nil {
		t.Fatal(err)
	}
	if machine.lastControl != "end" {
		t.Fatalf("end mapped to %q", machine.lastControl)
	}
	if err := backend.QueueInput(frontend.InputEvent{Control: "menu", Pressed: true}); err != nil {
		t.Fatal(err)
	}
	if machine.lastControl != "soft-left" {
		t.Fatalf("menu fallback mapped to %q", machine.lastControl)
	}
	if err := backend.QueueInput(frontend.InputEvent{Control: "not-a-board-control", Pressed: true}); err == nil {
		t.Fatal("unsupported board control was accepted")
	}
}

func TestBackendDefaultsToFastestRegisteredCPU(t *testing.T) {
	backend := NewBackend(Options{})
	if got := backend.options.CPUBackend; got != application.FastestBackend {
		t.Fatalf("default CPU backend = %q, want %q", got, application.FastestBackend)
	}
	precise := NewBackend(Options{CPUBackendMode: systemmachine.CPUBackendPrecise})
	if got := precise.options.CPUBackendMode; got != systemmachine.CPUBackendPrecise {
		t.Fatalf("explicit CPU backend = %q, want %q", got, systemmachine.CPUBackendPrecise)
	}
}

func TestBackendCPUSelectionUsesRegisteredFactory(t *testing.T) {
	backend := NewBackend(Options{})
	if err := backend.ConfigureCPU(frontend.CPUSettings{Name: "jit"}); err != nil {
		t.Fatal(err)
	}
	if backend.options.CPUBackend != "jit" || backend.options.CPUBackendMode != "" {
		t.Fatalf("configured CPU options = %+v", backend.options)
	}
	if err := backend.ConfigureCPU(frontend.CPUSettings{Name: "missing-system-core"}); err == nil {
		t.Fatal("unknown system CPU backend was accepted")
	}
}

func TestFirmwareContentIDIgnoresPieceOrder(t *testing.T) {
	first, err := firmwareset.NewSet([]firmwareset.Source{
		{ReaderAt: bytes.NewReader([]byte("alpha")), Size: 5},
		{ReaderAt: bytes.NewReader([]byte("beta")), Size: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := firmwareset.NewSet([]firmwareset.Source{
		{ReaderAt: bytes.NewReader([]byte("beta")), Size: 4},
		{ReaderAt: bytes.NewReader([]byte("alpha")), Size: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	if firmwareContentID(first) != firmwareContentID(second) {
		t.Fatal("content ID changed when firmware pieces were reordered")
	}
}

func TestMediaFileRoundTripAndChecksum(t *testing.T) {
	path := filepath.Join(t.TempDir(), "media.arammedia")
	want := systemmachine.MediaState{
		FirmwareBuildID: "samsung.sch-w830.dl21",
		Flash:           []byte("flash-state"),
		NAND:            []byte("nand-state"),
	}
	if err := writeMediaFile(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := readMediaFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.FirmwareBuildID != want.FirmwareBuildID ||
		!bytes.Equal(got.Flash, want.Flash) || !bytes.Equal(got.NAND, want.NAND) {
		t.Fatalf("media round trip = %+v", got)
	}
	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	encoded[len(encoded)-1] ^= 0xff
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readMediaFile(path); err == nil {
		t.Fatal("tampered media checksum was accepted")
	}
}

func TestBackendClosePersistsMediaBeforeClearingContentIdentity(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "phone.wbt"), []byte("boot"), 0o600); err != nil {
		t.Fatal(err)
	}
	mediaRoot := t.TempDir()
	machine := newFakeSystemMachine()
	backend := NewBackend(Options{
		MediaRoot: mediaRoot,
		newMachine: func(firmwareset.Set, systemmachine.Options) (systemMachine, error) {
			return machine, nil
		},
	})
	if _, err := backend.Open(context.Background(), frontend.OpenRequest{Path: directory}); err != nil {
		t.Fatal(err)
	}
	if err := backend.Close(); err != nil {
		t.Fatal(err)
	}
	mediaFiles, err := filepath.Glob(filepath.Join(mediaRoot, "*.arammedia"))
	if err != nil {
		t.Fatal(err)
	}
	if len(mediaFiles) != 1 {
		t.Fatalf("persistent media files = %v, want exactly one", mediaFiles)
	}
	got, err := readMediaFile(mediaFiles[0])
	if err != nil {
		t.Fatal(err)
	}
	if got.FirmwareBuildID != machine.media.FirmwareBuildID ||
		!bytes.Equal(got.Flash, machine.media.Flash) || !bytes.Equal(got.NAND, machine.media.NAND) {
		t.Fatalf("persisted close media = %+v, want %+v", got, machine.media)
	}
}

func TestPrivateSCHW830FirmwareReachesFrontendAdapter(t *testing.T) {
	directory := os.Getenv("ARAM_SCHW830_REFERENCE_DIR")
	if directory == "" {
		t.Skip("ARAM_SCHW830_REFERENCE_DIR is not configured")
	}
	backend := NewBackend(Options{
		InstructionsPerFrame:    1_195_629,
		DisableMediaPersistence: true,
	})
	t.Cleanup(func() { _ = backend.Close() })
	info, err := backend.Open(context.Background(), frontend.OpenRequest{
		Path:     directory,
		Firmware: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if info.ProfileID != "samsung.sch-w830.dl21" || backend.State() != frontend.StateReady {
		t.Fatalf("loaded input/state = %+v/%s", info, backend.State())
	}
	if err := backend.Execute(context.Background(), frontend.CommandStart); err != nil {
		t.Fatal(err)
	}
	if err := backend.RunFrame(context.Background()); err != nil {
		t.Fatal(err)
	}
	frame := backend.VideoFrame()
	if frame.Image == nil || frame.Image.Bounds().Dx() != 240 || frame.Image.Bounds().Dy() != 320 {
		t.Fatalf("frontend frame = %+v", frame)
	}
	if err := backend.QueueInput(frontend.InputEvent{Control: "soft-left", Pressed: true}); err != nil {
		t.Fatal(err)
	}
	if err := backend.QueueInput(frontend.InputEvent{Control: "soft-left", Pressed: false}); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateSCHW860FirmwareReachesFrontendAdapter(t *testing.T) {
	directory := os.Getenv("ARAM_SCHW860_DA06_DIR")
	if directory == "" {
		t.Skip("ARAM_SCHW860_DA06_DIR is not configured")
	}
	backend := NewBackend(Options{
		InstructionsPerFrame:    1,
		DisableMediaPersistence: true,
	})
	t.Cleanup(func() { _ = backend.Close() })
	info, err := backend.Open(context.Background(), frontend.OpenRequest{
		Path:     directory,
		Firmware: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if info.ProfileID != "samsung.sch-w860.da06" || backend.State() != frontend.StateReady {
		t.Fatalf("loaded input/state = %+v/%s", info, backend.State())
	}
	if err := backend.Execute(context.Background(), frontend.CommandStart); err != nil {
		t.Fatal(err)
	}
	if err := backend.RunFrame(context.Background()); err != nil {
		t.Fatal(err)
	}
	frame := backend.VideoFrame()
	if frame.Image == nil || frame.Image.Bounds().Dx() != 240 || frame.Image.Bounds().Dy() != 320 {
		t.Fatalf("frontend frame = %+v", frame)
	}
}
