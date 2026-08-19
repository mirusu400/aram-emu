package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mirusu400/aram-frontend/frontend"
)

// TestEPESaveDataProductRelaunch drives the whole product path for issue #53:
// open 에픽크로니클PE, confirm the first-run "restart required" notice (which
// calls MC_knlExit), close, then reopen and confirm the notice is skipped —
// proving the per-title save-data file carries gopt.sav across the relaunch.
// Gated on ARAM_EPE_REPRO.
func TestEPESaveDataProductRelaunch(t *testing.T) {
	gamePath := os.Getenv("ARAM_EPE_GAME")
	if gamePath == "" {
		t.Skip("ARAM_EPE_GAME is not set (path to 에픽크로니클PE.zip)")
	}
	if _, err := os.Stat(gamePath); err != nil {
		t.Skipf("game unavailable: %v", err)
	}
	stateRoot := t.TempDir()

	content := func(b *Backend) int {
		frame := b.VideoFrame()
		if frame.Image == nil {
			return 0
		}
		bounds := frame.Image.Bounds()
		count := 0
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				r, g, bl, _ := frame.Image.At(x, y).RGBA()
				if r|g|bl != 0 {
					count++
				}
			}
		}
		return count
	}

	open := func() *Backend {
		b := NewBackend(nil)
		b.stateRoot = stateRoot
		if _, err := b.Open(context.Background(), frontend.OpenRequest{Path: gamePath}); err != nil {
			t.Fatalf("open: %v", err)
		}
		if err := b.Execute(context.Background(), frontend.CommandStart); err != nil {
			t.Fatalf("start: %v", err)
		}
		return b
	}

	// --- Run 1: reach notice, confirm (MC_knlExit), close ---
	b1 := open()
	sha := b1.currentInputHash()
	for i := 0; i < 400; i++ {
		if err := b1.RunFrame(context.Background()); err != nil {
			t.Fatalf("run1 frame %d: %v", i, err)
		}
	}
	t.Logf("run1 at notice: state=%v content=%d", b1.State(), content(b1))
	_ = b1.QueueInput(frontend.InputEvent{Control: "select", Pressed: true})
	_ = b1.QueueInput(frontend.InputEvent{Control: "select", Pressed: false, At: 100 * time.Millisecond})
	for i := 0; i < 300 && b1.State() != frontend.StateStopped; i++ {
		if err := b1.RunFrame(context.Background()); err != nil {
			t.Fatalf("run1 post-key frame %d: %v", i, err)
		}
	}
	t.Logf("run1 after confirm: state=%v", b1.State())
	if err := b1.Close(); err != nil {
		t.Fatalf("run1 close: %v", err)
	}

	savePath := filepath.Join(stateRoot, sha, "savedata.bin")
	if info, err := os.Stat(savePath); err != nil {
		t.Fatalf("save-data file missing after run1: %v", err)
	} else {
		t.Logf("save-data file: %s (%d bytes)", savePath, info.Size())
	}

	// --- Run 2: reopen; the notice must be skipped ---
	b2 := open()
	defer b2.Close()
	for i := 0; i < 1000; i++ {
		if err := b2.RunFrame(context.Background()); err != nil {
			t.Fatalf("run2 frame %d: %v", i, err)
		}
	}
	c := content(b2)
	t.Logf("run2 after reopen: state=%v content=%d", b2.State(), c)
	if c < 20000 {
		t.Fatalf("run2 content=%d looks like the notice, not the title screen — save-data not restored", c)
	}
}
