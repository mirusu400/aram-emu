package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/mirusu400/aram-frontend/frontend"
)

func TestReferencePackageProducesAudiblePCM(t *testing.T) {
	path := os.Getenv("ARAM_AUDIO_PACKAGE")
	if path == "" {
		t.Skip("ARAM_AUDIO_PACKAGE is not set")
	}
	backend := NewBackend(nil)
	t.Cleanup(func() { _ = backend.Close() })
	if _, err := backend.Open(context.Background(), frontend.OpenRequest{
		Path: path,
	}); err != nil {
		t.Fatal(err)
	}
	if err := backend.Execute(
		context.Background(),
		frontend.CommandStart,
	); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(45 * time.Second)
	var sampleCount int
	var peak int16
	frames := 0
	for time.Now().Before(deadline) {
		if err := backend.RunFrame(context.Background()); err != nil {
			t.Fatal(err)
		}
		frames++
		audio := backend.DrainAudio()
		sampleCount += len(audio.PCM16)
		for _, sample := range audio.PCM16 {
			if sample < 0 {
				sample = -sample
			}
			if sample > peak {
				peak = sample
			}
		}
		if sampleCount >= 2_048 && peak >= 100 {
			t.Logf(
				"audible PCM: samples=%d peak=%d rate=%d channels=%d",
				sampleCount,
				peak,
				audio.SampleRate,
				audio.Channels,
			)
			return
		}
	}
	diagnostics := backend.Diagnostics()
	t.Fatalf(
		"package produced no audible PCM within deadline: frames=%d samples=%d peak=%d state=%s execution=%+v wipi=%+v",
		frames,
		sampleCount,
		peak,
		backend.State(),
		diagnostics.Execution,
		diagnostics.WIPI,
	)
}
