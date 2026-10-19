package integration

import (
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	"os"
	"testing"

	"github.com/mirusu400/aram-core/loader"
	"github.com/mirusu400/aram-frontend/frontend"
)

// TestPXO1OrdinaryProductPath uses a user-authorized local executable. Its
// bytes and path remain outside the repository.
func TestPXO1OrdinaryProductPath(t *testing.T) {
	path := os.Getenv("ARAM_PXO_DATA")
	if path == "" {
		t.Skip("ARAM_PXO_DATA is not set")
	}
	frames := make(map[string]string)
	for _, test := range []struct {
		name string
		down bool
	}{
		{name: "sanakan"},
		{name: "3d-block", down: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			backend := NewBackend(nil)
			t.Cleanup(func() { _ = backend.Close() })
			info, err := backend.Open(context.Background(), frontend.OpenRequest{Path: path})
			if err != nil {
				t.Fatal(err)
			}
			if info.Format != string(loader.KindPXO) {
				t.Fatalf("product format = %q, want %q", info.Format, loader.KindPXO)
			}
			if err := backend.Execute(context.Background(), frontend.CommandStart); err != nil {
				t.Fatal(err)
			}
			if test.down {
				if err := backend.QueueInput(frontend.InputEvent{Control: "down", Pressed: true}); err != nil {
					t.Fatal(err)
				}
			}
			if err := backend.QueueInput(frontend.InputEvent{Control: "select", Pressed: true}); err != nil {
				t.Fatal(err)
			}
			if err := backend.RunFrame(context.Background()); err != nil {
				t.Fatal(err)
			}
			diagnostics := backend.Diagnostics().PXO
			if diagnostics == nil || diagnostics.PresentCount == 0 ||
				!diagnostics.FrameValid || !diagnostics.NonUniform {
				t.Fatalf("PXO1 product diagnostics = %+v", diagnostics)
			}
			frame := backend.VideoFrame().Image
			if frame == nil || frame.Bounds() != image.Rect(0, 0, 176, 220) {
				t.Fatalf("product frame bounds = %v", frame)
			}
			rgba, ok := frame.(*image.RGBA)
			if !ok {
				t.Fatalf("product frame type = %T, want *image.RGBA", frame)
			}
			frames[test.name] = fmt.Sprintf("%x", sha256.Sum256(rgba.Pix))
		})
	}
	if frames["sanakan"] == frames["3d-block"] {
		t.Fatalf("both game selections rendered the same product frame %s", frames["sanakan"])
	}
}
