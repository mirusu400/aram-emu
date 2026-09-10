package main

import "testing"

func TestProbeRequestsStableStereoOutput(t *testing.T) {
	for _, mix := range []bool{false, true} {
		settings := probeAudioSettings(mix)
		if settings.OutputSampleRate != 44100 || settings.OutputChannels != 2 {
			t.Fatalf("probe output must request 44100 Hz stereo, got %+v", settings)
		}
		if settings.MixMode != mix {
			t.Fatalf("audio policy changed: got mix=%v, want %v", settings.MixMode, mix)
		}
	}
}
