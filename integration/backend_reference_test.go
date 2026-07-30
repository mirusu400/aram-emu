package integration

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mirusu400/aram-core/loader/raptor"
	"github.com/mirusu400/aram-frontend/frontend"
)

func TestReferenceRaptorRunsContinuouslyThroughFrontend(t *testing.T) {
	root := os.Getenv("ARAM_TEST_DATA")
	if root == "" {
		t.Skip("ARAM_TEST_DATA is not set")
	}
	path := findReferenceRaptor(t, root)
	temporary := t.TempDir()
	t.Setenv("APPDATA", temporary)
	t.Setenv("XDG_CONFIG_HOME", temporary)

	backend := NewBackend(nil)
	t.Cleanup(func() { _ = backend.Close() })
	shell := frontend.NewShell(backend, unavailablePicker{}, path)
	waitForFrontendState(t, shell, backend, frontend.StateReady, 15*time.Second)

	shell.DispatchExternalCommand("emu.start")
	deadline := time.Now().Add(45 * time.Second)
	var (
		baselineCalls uint64
		sawRunning    bool
	)
	for time.Now().Before(deadline) {
		if err := shell.Update(); err != nil {
			t.Fatal(err)
		}
		diagnostics := backend.Diagnostics()
		if backend.State() == frontend.StateRunning && diagnostics.WIPI != nil {
			if !sawRunning {
				baselineCalls = diagnostics.WIPI.APICalls
				sawRunning = true
			} else if diagnostics.WIPI.APICalls > baselineCalls {
				return
			}
		}
		time.Sleep(time.Millisecond)
	}
	diagnostics := backend.Diagnostics()
	t.Fatalf(
		"frontend did not continuously advance %s: running=%t baseline_calls=%d diagnostics=%+v",
		path,
		sawRunning,
		baselineCalls,
		diagnostics,
	)
}

func waitForFrontendState(
	t *testing.T,
	shell *frontend.Shell,
	backend *Backend,
	want frontend.BackendState,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := shell.Update(); err != nil {
			t.Fatal(err)
		}
		if backend.State() == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("frontend state = %s, want %s", backend.State(), want)
}

func findReferenceRaptor(t *testing.T, root string) string {
	t.Helper()
	var selected string
	stop := errors.New("Raptor package selected")
	err := filepath.WalkDir(root, func(
		path string,
		entry os.DirEntry,
		walkErr error,
	) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".zip") {
			return nil
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if _, err := raptor.Inspect(payload); err != nil {
			return nil
		}
		selected = path
		return stop
	})
	if err != nil && !errors.Is(err, stop) {
		t.Fatal(err)
	}
	if selected == "" {
		t.Fatal("ARAM_TEST_DATA contained no valid Raptor package")
	}
	return selected
}
