package integration

import (
	"testing"

	"github.com/mirusu400/aram-core/application"
)

func TestDefaultBackendUsesNativeHandsetFrameBudget(t *testing.T) {
	backend := NewBackend(nil)
	factory, ok := backend.factory.(application.Factory)
	if !ok {
		t.Fatalf("default factory type = %T", backend.factory)
	}
	if factory.FrameRunBudget != application.DefaultHandsetRunBudget {
		t.Fatalf(
			"default generic frame run budget = %d, want %d",
			factory.FrameRunBudget,
			application.DefaultHandsetRunBudget,
		)
	}
}
