package integration

import (
	"testing"

	aramcore "github.com/mirusu400/aram-core/core"
)

type publishedAudioMachine struct {
	aramcore.Machine
	chunk aramcore.AudioChunk
	calls int
}

func (m *publishedAudioMachine) DrainPublishedAudio() aramcore.AudioChunk {
	m.calls++
	return m.chunk
}

func TestDrainAudioUsesPublishedQueueWhileFrameOperationIsLocked(t *testing.T) {
	inner := &publishedAudioMachine{chunk: aramcore.AudioChunk{
		SampleRate:   44_100,
		Channels:     2,
		PCM16:        []int16{1, -1, 2, -2},
		StartGuestNS: 20_000_000,
		StartSample:  882,
		Generation:   6,
	}}
	wrapper := &unwrappingFrameMachine{Machine: inner}
	backend := &Backend{machine: wrapper}

	backend.operationMu.Lock()
	defer backend.operationMu.Unlock()
	chunk := backend.DrainAudio()
	if inner.calls != 1 || len(chunk.PCM16) != 4 {
		t.Fatalf("published drain = %+v, calls=%d", chunk, inner.calls)
	}
	if chunk.StartGuestNS != 20_000_000 ||
		chunk.StartSample != 882 ||
		chunk.Generation != 6 {
		t.Fatalf("published audio timeline = %+v", chunk)
	}
}

func TestDrainAudioKeepsSerializedFallbackForLegacyMachines(t *testing.T) {
	legacy := &unwrappingFrameMachine{}
	backend := &Backend{machine: legacy}
	backend.operationMu.Lock()
	defer backend.operationMu.Unlock()
	if chunk := backend.DrainAudio(); len(chunk.PCM16) != 0 {
		t.Fatalf("legacy drain bypassed operation lock: %+v", chunk)
	}
}
