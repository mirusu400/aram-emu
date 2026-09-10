package integration

import (
	"context"
	"testing"

	"github.com/mirusu400/aram-frontend/frontend"
)

func TestJavaDiagnosticsPollingReusesVerifiedCoreFrame(t *testing.T) {
	ctx := context.Background()
	backend := NewBackend(nil)
	if err := backend.ConfigureStateRoot(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	if _, err := backend.OpenWithProgress(ctx, frontend.OpenRequest{
		DisplayName: "diagnostics.jar", Data: syntheticJ2ME(t, false),
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := backend.Execute(ctx, frontend.CommandStart); err != nil {
		t.Fatal(err)
	}
	first := backend.Diagnostics().Java
	if first == nil || first.HasDisplay || !first.Started || first.Instructions != 4 ||
		first.PresentCount != 1 || !first.FrameValid || first.FramebufferSHA256 == "" {
		t.Fatalf("idle Java diagnostics = %+v", first)
	}
	// This is the same exported read-only path used twice per probe slice.
	// Count objects, not elapsed time, to detect full snapshots/pixel copies.
	allocations := testing.AllocsPerRun(100, func() {
		if got := backend.Diagnostics().Java; got == nil || *got != *first {
			panic("diagnostics changed without execution")
		}
	})
	if allocations > 4 {
		t.Fatalf("Java diagnostic polling allocates %.0f objects, want at most 4", allocations)
	}
	for range 16 {
		if err := backend.RunFrame(ctx); err != nil {
			t.Fatal(err)
		}
		_ = backend.Diagnostics()
	}
	next := backend.Diagnostics().Java
	if next.PresentCount != first.PresentCount+16 || next.Instructions != first.Instructions ||
		next.FramebufferSHA256 != first.FramebufferSHA256 || !next.FrameValid || next.HasDisplay {
		t.Fatalf("full idle execution changed diagnostics contract: before=%+v after=%+v", first, next)
	}
}
