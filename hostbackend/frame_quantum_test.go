package hostbackend

import (
	"context"
	"testing"
	"time"

	"github.com/mirusu400/aram-frontend/frontend"
)

type quantumAdapter struct {
	recordingAdapter
	quantum time.Duration
}

func (a *quantumAdapter) FrameQuantum() time.Duration { return a.quantum }

func TestFrameQuantumTracksActiveAdapterAndRuntimeChanges(t *testing.T) {
	application := &quantumAdapter{quantum: 16 * time.Millisecond}
	system := &quantumAdapter{quantum: 20 * time.Millisecond}
	backend := newBackend(application, system)
	var reporter frontend.FrameQuantumBackend = backend
	if got := reporter.FrameQuantum(); got != application.quantum {
		t.Fatalf("application quantum = %s, want %s", got, application.quantum)
	}
	application.quantum = 8 * time.Millisecond
	if got := reporter.FrameQuantum(); got != application.quantum {
		t.Fatalf("changed runtime quantum = %s, want %s", got, application.quantum)
	}
	if _, err := backend.Open(context.Background(), frontend.OpenRequest{Firmware: true}); err != nil {
		t.Fatal(err)
	}
	if got := reporter.FrameQuantum(); got != system.quantum {
		t.Fatalf("system quantum = %s, want %s", got, system.quantum)
	}
	if _, err := backend.Open(context.Background(), frontend.OpenRequest{Path: "synthetic.zip"}); err != nil {
		t.Fatal(err)
	}
	if got := reporter.FrameQuantum(); got != application.quantum {
		t.Fatalf("reopened application quantum = %s, want %s", got, application.quantum)
	}
}

func TestFrameQuantumPreservesFallbackForUnreportedOrInvalidQuantum(t *testing.T) {
	for _, application := range []frontend.Backend{
		&recordingAdapter{},
		&quantumAdapter{},
		&quantumAdapter{quantum: -time.Millisecond},
	} {
		backend := newBackend(application, &recordingAdapter{})
		if got := backend.FrameQuantum(); got != time.Second/60 {
			t.Fatalf("fallback quantum = %s, want %s", got, time.Second/60)
		}
	}
}
