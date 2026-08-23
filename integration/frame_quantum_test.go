package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mirusu400/aram-frontend/frontend"
)

// ktfPresentationQuantum mirrors aram-core's KTF presentation quantum. The core
// constant lives in an internal package, so the contract is pinned by value:
// a KTF title paced at the native-WIPI fallback of sixteen milliseconds runs
// four percent fast.
const ktfPresentationQuantum = (time.Second + 30) / 60

func TestFrameQuantumReadsThroughTheCheatWrapper(t *testing.T) {
	jar := syntheticZIP(t, map[string][]byte{
		"client.bin4096": syntheticKTFBootstrapClient(),
	})
	archive := syntheticZIP(t, map[string][]byte{
		"01020304.jar": jar,
		"__adf__": []byte(
			"PID:PD000001\nAID:01020304\nMClass:GameMain\n",
		),
	})
	path := filepath.Join(t.TempDir(), "synthetic.zip")
	if err := os.WriteFile(path, archive, 0o600); err != nil {
		t.Fatal(err)
	}

	backend := NewBackend(nil)
	t.Cleanup(func() { _ = backend.Close() })
	if quantum := backend.FrameQuantum(); quantum != defaultFrameQuantum {
		t.Fatalf("frame quantum with no machine = %s, want %s", quantum, defaultFrameQuantum)
	}
	if _, err := backend.Open(
		context.Background(),
		frontend.OpenRequest{Path: path},
	); err != nil {
		t.Fatal(err)
	}
	if quantum := backend.FrameQuantum(); quantum != ktfPresentationQuantum {
		t.Fatalf("KTF frame quantum = %s, want %s", quantum, ktfPresentationQuantum)
	}
}
