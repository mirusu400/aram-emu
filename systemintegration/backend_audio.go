package systemintegration

import (
	aramcore "github.com/mirusu400/aram-core/core"
	"github.com/mirusu400/aram-frontend/frontend"
)

// DrainAudio connects the whole-phone machine's native firmware PCM to the
// same frontend stream consumed by application mode. TryLock keeps the audio
// pump non-blocking while a CPU frame or lifecycle operation owns the machine.
func (backend *Backend) DrainAudio() frontend.AudioChunk {
	if !backend.operationMu.TryLock() {
		return frontend.AudioChunk{}
	}
	defer backend.operationMu.Unlock()

	machine := backend.currentMachine()
	if machine == nil {
		return frontend.AudioChunk{}
	}
	audio, ok := machine.(interface {
		DrainAudio() aramcore.AudioChunk
	})
	if !ok {
		return frontend.AudioChunk{}
	}
	chunk := audio.DrainAudio()
	return frontend.AudioChunk{
		SampleRate:   chunk.SampleRate,
		Channels:     chunk.Channels,
		PCM16:        chunk.PCM16,
		StartGuestNS: chunk.StartGuestNS,
		StartSample:  chunk.StartSample,
		Generation:   chunk.Generation,
	}
}

var _ frontend.AudioStreamBackend = (*Backend)(nil)
