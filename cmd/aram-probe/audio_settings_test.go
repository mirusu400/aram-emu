package main

import "testing"

func TestProbeRequestsStableMonoOutput(t *testing.T) {
	for _, mix := range []bool{false, true} {
		settings := probeAudioSettings(mix)
		if settings.OutputSampleRate != 44100 || settings.OutputChannels != 1 {
			t.Fatalf("probe output must match the ordinary 44100 Hz mono default, got %+v", settings)
		}
		if settings.MixMode != mix {
			t.Fatalf("audio policy changed: got mix=%v, want %v", settings.MixMode, mix)
		}
	}
}
