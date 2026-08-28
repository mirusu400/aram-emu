package hostbackend

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mirusu400/aram-frontend/frontend"
)

// firmwarePicker answers the shell's firmware chooser with a fixed directory
// and refuses every other selection, so a test can only reach the firmware
// path it means to exercise.
type firmwarePicker struct{ directory string }

func (p firmwarePicker) OpenFile() (string, error) { return "", frontend.ErrPickerCanceled }

func (p firmwarePicker) OpenFontFile() (string, error) { return "", frontend.ErrPickerCanceled }

func (p firmwarePicker) OpenFirmwareDirectory(string) (string, error) { return p.directory, nil }

func (p firmwarePicker) OpenSaveBackupFile() (string, error) { return "", frontend.ErrPickerCanceled }

func (p firmwarePicker) ChooseRecent([]string) (string, error) {
	return "", frontend.ErrPickerCanceled
}

// The shipping product opens firmware through its ordinary shell. Before the
// two adapters were routed behind one backend, this command reached the
// application adapter and could only fail, so the test drives the same command
// a person does rather than calling the whole-phone adapter directly.
func TestReferenceFirmwareOpensThroughTheProductShell(t *testing.T) {
	reference := os.Getenv("ARAM_REFERENCE_REPO")
	if reference == "" {
		t.Skip("ARAM_REFERENCE_REPO is not set")
	}
	directory := filepath.Join(reference, "SCH-W380_DL21")
	if _, err := os.Stat(directory); err != nil {
		t.Skipf("reference firmware directory is unavailable: %v", err)
	}

	temporary := t.TempDir()
	t.Setenv("APPDATA", temporary)
	t.Setenv("XDG_CONFIG_HOME", temporary)

	backend := NewBackend(Options{System: DefaultSystemOptions()})
	defer backend.Close()
	if !backend.SupportsFirmware() {
		t.Fatal("the product backend did not advertise firmware support")
	}

	applicationName := backend.BackendName()
	shell := frontend.NewShell(backend, firmwarePicker{directory: directory}, "")
	shell.DispatchExternalCommand("file.open_firmware")

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if err := shell.Update(); err != nil {
			t.Fatal(err)
		}
		if backend.State() != frontend.StateEmpty {
			break
		}
	}
	if state := backend.State(); state == frontend.StateEmpty || state == frontend.StateFaulted {
		t.Fatalf("firmware did not load through the product shell: state %q", state)
	}
	systemName := backend.BackendName()
	if systemName == applicationName {
		t.Fatalf("firmware stayed on the application adapter %q", applicationName)
	}
	t.Logf("firmware opened on adapter %q in state %q (application adapter is %q)",
		systemName, backend.State(), applicationName)
}
