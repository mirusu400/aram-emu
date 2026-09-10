package main

import "github.com/mirusu400/aram-frontend/frontend"

// probeAudioSettings makes the headless output contract independent of a
// title's native mono/stereo layout. The backend performs actual conversion.
func probeAudioSettings(mix bool) frontend.AudioSettings {
	return frontend.AudioSettings{MixMode: mix, OutputSampleRate: 44100, OutputChannels: 2}
}
