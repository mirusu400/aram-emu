package integration

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mirusu400/aram-frontend/frontend"
)

func TestOrdinaryOpenMapsAndExecutesNativeEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic.dat")
	if err := os.WriteFile(path, syntheticEADS(), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := NewBackend(nil)
	t.Cleanup(func() { _ = backend.Close() })

	var stages []frontend.OpenStage
	info, err := backend.OpenWithProgress(
		context.Background(),
		frontend.OpenRequest{Path: path},
		func(stage frontend.OpenStage) {
			stages = append(stages, stage)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(stages) != 2 ||
		stages[0] != frontend.OpenStageInspecting ||
		stages[1] != frontend.OpenStageLoading {
		t.Fatalf("open stages = %v", stages)
	}
	if info.DisplayName != "synthetic.dat" ||
		info.Format != "eads" ||
		info.ProfileID != "wipi-1.2.1/generic" ||
		len(info.SHA256) != 64 {
		t.Fatalf("input info = %+v", info)
	}
	if backend.State() != frontend.StateReady {
		t.Fatalf("state after open = %s", backend.State())
	}
	if capability := backend.Capability(frontend.CommandStart); !capability.Supported {
		t.Fatalf("start capability = %+v", capability)
	}

	if err := backend.Execute(context.Background(), frontend.CommandStart); err != nil {
		t.Fatal(err)
	}
	if backend.State() != frontend.StatePaused {
		t.Fatalf("state after entry slice = %s", backend.State())
	}
	diagnostics := backend.Diagnostics()
	if diagnostics.Image == nil ||
		diagnostics.Image.Name != "SyntheticEADS" ||
		diagnostics.Image.EntryPoint != 0x02000001 ||
		diagnostics.Execution == nil ||
		diagnostics.Execution.Reason != "budget" ||
		diagnostics.Execution.Instructions != 1 ||
		diagnostics.Execution.PC != 0x02000002 {
		t.Fatalf("diagnostics after start = %+v", diagnostics)
	}
	if diagnostics.WIPI == nil ||
		diagnostics.WIPI.CatalogedAPIs != 239 ||
		diagnostics.WIPI.DispatchWiredAPIs != 239 ||
		diagnostics.WIPI.SemanticallyModeled != 239 ||
		diagnostics.WIPI.PresentCount != 0 {
		t.Fatalf("public WIPI diagnostics after start = %+v", diagnostics.WIPI)
	}
	frame := backend.VideoFrame()
	if frame.Image == nil || frame.Image.Bounds().Dx() != 240 || frame.Image.Bounds().Dy() != 320 {
		t.Fatalf("video frame = %+v", frame)
	}
	debugger, err := backend.ToolSnapshot(context.Background(), frontend.ToolDebugger)
	if err != nil {
		t.Fatal(err)
	}
	if len(debugger.Lines) < 3 {
		t.Fatalf("debugger snapshot = %+v", debugger)
	}
}

type unavailablePicker struct{}

func (unavailablePicker) OpenFile() (string, error) {
	return "", frontend.ErrPickerUnavailable
}

func (unavailablePicker) OpenFirmwareDirectory(string) (string, error) {
	return "", frontend.ErrPickerUnavailable
}

func (unavailablePicker) ChooseRecent([]string) (string, error) {
	return "", frontend.ErrPickerUnavailable
}

type pathPicker struct {
	path string
}

func (picker pathPicker) OpenFile() (string, error) {
	return picker.path, nil
}

func (pathPicker) OpenFirmwareDirectory(string) (string, error) {
	return "", frontend.ErrPickerUnavailable
}

func (pathPicker) ChooseRecent([]string) (string, error) {
	return "", frontend.ErrPickerUnavailable
}

func TestFrontendInitialPathConvergesOnIntegratedOpen(t *testing.T) {
	temporary := t.TempDir()
	t.Setenv("APPDATA", temporary)
	t.Setenv("XDG_CONFIG_HOME", temporary)
	path := filepath.Join(temporary, "initial.dat")
	if err := os.WriteFile(path, syntheticEADS(), 0o600); err != nil {
		t.Fatal(err)
	}

	backend := NewBackend(nil)
	defer backend.Close()
	shell := frontend.NewShell(backend, unavailablePicker{}, path)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := shell.Update(); err != nil {
			t.Fatal(err)
		}
		if backend.State() == frontend.StateReady {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("frontend initial path did not load; backend state = %s", backend.State())
}

func TestFrontendFileOpenCommandReachesIntegratedEntry(t *testing.T) {
	temporary := t.TempDir()
	t.Setenv("APPDATA", temporary)
	t.Setenv("XDG_CONFIG_HOME", temporary)
	path := filepath.Join(temporary, "file-open.dat")
	if err := os.WriteFile(path, syntheticEADS(), 0o600); err != nil {
		t.Fatal(err)
	}

	backend := NewBackend(nil)
	defer backend.Close()
	shell := frontend.NewShell(backend, pathPicker{path: path}, "")
	shell.DispatchExternalCommand("file.open")

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := shell.Update(); err != nil {
			t.Fatal(err)
		}
		if backend.State() == frontend.StateReady {
			if err := backend.Execute(context.Background(), frontend.CommandStart); err != nil {
				t.Fatal(err)
			}
			if backend.State() != frontend.StatePaused {
				t.Fatalf("state after entry = %s", backend.State())
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("frontend File/Open did not load; backend state = %s", backend.State())
}

func TestFirmwareOpenStaysExplicitlyUnsupported(t *testing.T) {
	backend := NewBackend(nil)
	_, err := backend.Open(context.Background(), frontend.OpenRequest{
		Path:     "firmware",
		Firmware: true,
	})
	if err == nil {
		t.Fatal("firmware Open unexpectedly succeeded")
	}
	backendErr, ok := err.(*frontend.BackendError)
	if !ok || backendErr.Kind != frontend.FailureUnsupportedProfile {
		t.Fatalf("Open error = %#v", err)
	}
}

func TestOpenDistinguishesUnsupportedFormatFromMalformedDAT(t *testing.T) {
	temporary := t.TempDir()
	tests := []struct {
		name     string
		data     []byte
		wantKind frontend.FailureKind
	}{
		{
			name:     "unsupported.jar",
			data:     []byte("PK\x03\x04synthetic Java archive placeholder"),
			wantKind: frontend.FailureUnsupportedProfile,
		},
		{
			name:     "malformed.dat",
			data:     []byte("synthetic malformed WIPI container"),
			wantKind: frontend.FailureMalformedInput,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(temporary, test.name)
			if err := os.WriteFile(path, test.data, 0o600); err != nil {
				t.Fatal(err)
			}
			backend := NewBackend(nil)
			t.Cleanup(func() { _ = backend.Close() })
			_, err := backend.Open(
				context.Background(),
				frontend.OpenRequest{Path: path},
			)
			var backendErr *frontend.BackendError
			if !errors.As(err, &backendErr) || backendErr.Kind != test.wantKind {
				t.Fatalf("Open() error = %v; want backend kind %q", err, test.wantKind)
			}
		})
	}
}

func TestMagicholeReferenceUsesOrdinaryProductPath(t *testing.T) {
	reference := os.Getenv("ARAM_REFERENCE_REPO")
	if reference == "" {
		t.Skip("ARAM_REFERENCE_REPO is not set")
	}
	path := filepath.Join(
		reference,
		"SCH-W380_DL21",
		"SCH-W830_DL21_29360128_DL21.dat",
	)
	if _, err := os.Stat(path); err != nil {
		t.Skipf("reference DAT is unavailable: %v", err)
	}

	temporary := t.TempDir()
	t.Setenv("APPDATA", temporary)
	t.Setenv("XDG_CONFIG_HOME", temporary)
	backend := NewBackend(nil)
	defer backend.Close()
	shell := frontend.NewShell(backend, pathPicker{path: path}, "")
	shell.DispatchExternalCommand("file.open")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := shell.Update(); err != nil {
			t.Fatal(err)
		}
		if backend.State() == frontend.StateReady {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if backend.State() != frontend.StateReady {
		t.Fatalf("reference File/Open state = %s", backend.State())
	}
	backend.mu.RLock()
	info := backend.input
	backend.mu.RUnlock()
	if info.Format != "eads" ||
		info.ProfileID != "wipi-1.2.1/skt/samsung/sch-w830/minigame-qvga-oem" ||
		len(info.SHA256) != 64 {
		t.Fatalf("reference input info = %+v", info)
	}
	if err := backend.Execute(context.Background(), frontend.CommandStart); err != nil {
		t.Fatal(err)
	}
	diagnostics := backend.Diagnostics()
	if diagnostics.EADS == nil ||
		len(diagnostics.EADS.Events) != 5 ||
		diagnostics.EADS.PresentCount != 2 ||
		diagnostics.EADS.TickMS != 32 {
		t.Fatalf("reference EADS diagnostics = %+v", diagnostics.EADS)
	}
	debugger, err := backend.ToolSnapshot(context.Background(), frontend.ToolDebugger)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(debugger.Lines, "\n")
	if !strings.Contains(joined, "MinigameQVGAOEM") ||
		!strings.Contains(joined, "0xf4000001 (Thumb)") {
		t.Fatalf("debugger snapshot:\n%s", joined)
	}
}

func syntheticEADS() []byte {
	const offset = 0x80
	data := make([]byte, offset+0x30+4)
	copy(data[offset:], "EADS")
	binary.LittleEndian.PutUint32(data[offset+4:], 1)
	binary.LittleEndian.PutUint32(data[offset+8:], 1)
	binary.LittleEndian.PutUint32(data[offset+12:], 0x02000000)
	binary.LittleEndian.PutUint32(data[offset+16:], 4)
	binary.LittleEndian.PutUint32(data[offset+20:], 0x03000000)
	binary.LittleEndian.PutUint32(data[offset+24:], 0x1000)
	copy(data[offset+0x20:], "SyntheticEADS")
	copy(data[offset+0x30:], []byte{
		0x00, 0xb5,
		0x00, 0xbe,
	})
	return data
}
