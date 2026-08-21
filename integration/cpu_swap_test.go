package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mirusu400/aram-core/application"
	"github.com/mirusu400/aram-core/cpu"
	"github.com/mirusu400/aram-core/cpu/interpreter"
	"github.com/mirusu400/aram-frontend/frontend"
)

// swapCPU is the interpreter under a distinct backend identity — a stand-in for
// a fast core so the backend swap is observable end-to-end without a second real
// engine. It reproduces the interpreter exactly (validated by cpu/conformance).
type swapCPU struct {
	*interpreter.Backend
}

func (swapCPU) Identity() cpu.Identity {
	return cpu.Identity{Name: "swap-demo-core", Version: "1", Architecture: cpu.ARMv5TE}
}

// TestARAMCPUSwapIsObservableThroughProduct proves the ARAM_CPU selection flows
// all the way through the product: registering a backend and pointing ARAM_CPU
// at it makes the ordinary NewBackend(nil) product path run on that core, and
// the active backend surfaces in the diagnostics the probe and frontend read.
func TestARAMCPUSwapIsObservableThroughProduct(t *testing.T) {
	application.RegisterCPUBackend("swap-demo", func() cpu.Backend {
		return swapCPU{interpreter.New()}
	})

	run := func(t *testing.T, aramCPU string) string {
		t.Helper()
		if aramCPU == "" {
			t.Setenv("ARAM_CPU", "")
			os.Unsetenv("ARAM_CPU")
		} else {
			t.Setenv("ARAM_CPU", aramCPU)
		}
		path := filepath.Join(t.TempDir(), "synthetic.dat")
		if err := os.WriteFile(path, syntheticEADS(), 0o600); err != nil {
			t.Fatal(err)
		}
		backend := NewBackend(nil)
		t.Cleanup(func() { _ = backend.Close() })
		if _, err := backend.Open(context.Background(), frontend.OpenRequest{Path: path}); err != nil {
			t.Fatal(err)
		}
		if err := backend.Execute(context.Background(), frontend.CommandStart); err != nil {
			t.Fatal(err)
		}
		diag := backend.Diagnostics()
		if diag.Image == nil {
			t.Fatal("no image diagnostics")
		}
		return diag.Image.CPUBackend
	}

	if got := run(t, ""); got != interpreter.BackendName {
		t.Fatalf("default backend = %q, want %q", got, interpreter.BackendName)
	}
	if got := run(t, "precise"); got != interpreter.BackendName {
		t.Fatalf("precise backend = %q, want %q", got, interpreter.BackendName)
	}
	if got := run(t, "swap-demo"); got != "swap-demo-core" {
		t.Fatalf("swapped backend = %q, want swap-demo-core", got)
	}
	// An unknown backend must fall back to the interpreter, not fail the product.
	if got := run(t, "no-such-native-core"); got != interpreter.BackendName {
		t.Fatalf("unknown backend fallback = %q, want %q", got, interpreter.BackendName)
	}
}
