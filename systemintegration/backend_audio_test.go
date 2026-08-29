package systemintegration

import (
	"testing"

	aramcore "github.com/mirusu400/aram-core/core"
)

type audioSystemMachine struct {
	*fakeSystemMachine
	audio aramcore.AudioChunk
}

func (machine *audioSystemMachine) DrainAudio() aramcore.AudioChunk {
	chunk := machine.audio
	machine.audio = aramcore.AudioChunk{}
	return chunk
}

func TestBackendDrainsSystemMachineAudioWithoutBlockingFrames(t *testing.T) {
	machine := &audioSystemMachine{
		fakeSystemMachine: newFakeSystemMachine(),
		audio: aramcore.AudioChunk{
			SampleRate: 44_100, Channels: 2, PCM16: []int16{1, -2, 3, -4},
			StartGuestNS: 25_000_000, StartSample: 1102, Generation: 3,
		},
	}
	backend := NewBackend(Options{})
	backend.machine = machine

	chunk := backend.DrainAudio()
	if chunk.SampleRate != 44_100 || chunk.Channels != 2 ||
		len(chunk.PCM16) != 4 || chunk.PCM16[1] != -2 ||
		chunk.StartGuestNS != 25_000_000 || chunk.StartSample != 1102 ||
		chunk.Generation != 3 {
		t.Fatalf("frontend audio chunk = %+v", chunk)
	}
	if second := backend.DrainAudio(); len(second.PCM16) != 0 {
		t.Fatalf("second drain returned %d samples", len(second.PCM16))
	}

	backend.operationMu.Lock()
	if busy := backend.DrainAudio(); len(busy.PCM16) != 0 {
		t.Fatalf("busy drain returned %d samples", len(busy.PCM16))
	}
	backend.operationMu.Unlock()
}

func TestBackendAudioIsOptionalForLegacySystemMachines(t *testing.T) {
	backend := NewBackend(Options{})
	backend.machine = newFakeSystemMachine()
	if chunk := backend.DrainAudio(); len(chunk.PCM16) != 0 {
		t.Fatalf("legacy machine returned %d samples", len(chunk.PCM16))
	}
}
