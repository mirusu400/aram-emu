package main

import "github.com/mirusu400/aram-frontend/frontend"

// probeAudioSettings matches the ordinary product's 44.1 kHz mono default.
// The backend converts a title's native layout without changing mix policy.
func probeAudioSettings(mix bool) frontend.AudioSettings {
	return frontend.AudioSettings{MixMode: mix, OutputSampleRate: 44100, OutputChannels: 1}
}
