package integration

import (
	"context"
	"testing"

	"github.com/mirusu400/aram-frontend/frontend"
)

func TestJ2MEStartResumesPausedBackend(t *testing.T) {
	ctx := context.Background()
	backend := NewBackend(nil)
	if err := backend.ConfigureStateRoot(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	if _, err := backend.Open(ctx, frontend.OpenRequest{DisplayName: "resume.jar", Data: syntheticJ2ME(t, false)}); err != nil {
		t.Fatal(err)
	}
	execute := func(command frontend.BackendCommand) {
		t.Helper()
		if capability := backend.Capability(command); !capability.Supported {
			t.Fatalf("%s not advertised: %+v", command, capability)
		}
		if err := backend.Execute(ctx, command); err != nil {
			t.Fatalf("%s: %v", command, err)
		}
	}
	state := func(want frontend.BackendState) {
		t.Helper()
		if got := backend.State(); got != want {
			t.Fatalf("state = %s, want %s", got, want)
		}
	}
	state(frontend.StateReady)
	execute(frontend.CommandStart)
	state(frontend.StateRunning)
	for cycle := 0; cycle < 2; cycle++ {
		execute(frontend.CommandPauseResume)
		state(frontend.StatePaused)
		before, ok := backend.CoreDebugSnapshot(1)
		if !ok || before.SKVM == nil || !before.SKVM.Started {
			t.Fatalf("missing started Java snapshot: %+v", before)
		}
		execute(frontend.CommandStart)
		state(frontend.StateRunning)
		after, ok := backend.CoreDebugSnapshot(1)
		if !ok || after.SKVM == nil || !after.SKVM.Started || after.SKVM.Instructions != before.SKVM.Instructions {
			t.Fatalf("resume reran Java initialization: before=%+v after=%+v", before.SKVM, after.SKVM)
		}
		if backend.Supports(frontend.CommandStart) {
			t.Fatal("Start advertised while running")
		}
		if err := backend.Execute(ctx, frontend.CommandStart); err == nil {
			t.Fatal("Start while running succeeded")
		}
		state(frontend.StateRunning)
		if err := backend.RunFrame(ctx); err != nil {
			t.Fatal(err)
		}
		state(frontend.StateRunning)
	}
	execute(frontend.CommandStop)
	state(frontend.StateStopped)
	execute(frontend.CommandStart)
	state(frontend.StateRunning)
	execute(frontend.CommandReset)
	state(frontend.StateReady)
	execute(frontend.CommandStart)
	state(frontend.StateRunning)
}
