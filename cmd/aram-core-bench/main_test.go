package main

import (
	"crypto/sha256"
	"testing"

	aramcore "github.com/mirusu400/aram-core/core"
	"github.com/mirusu400/aram-emu/internal/framebench"
)

func TestStoppedGuestCannotBecomeAFalseFastBenchmark(t *testing.T) {
	for _, state := range []aramcore.State{aramcore.StateEmpty, aramcore.StateReady, aramcore.StateStopped, aramcore.StateFaulted} {
		if canContinue(state) {
			t.Fatalf("benchmark would count non-executing state %s", state)
		}
	}
	for _, state := range []aramcore.State{aramcore.StateRunning, aramcore.StatePaused} {
		if !canContinue(state) {
			t.Fatalf("valid deterministic frame yield %s rejected", state)
		}
	}
}

type publishedTestMachine struct {
	aramcore.Machine
	chunks []aramcore.AudioChunk
}

func (*publishedTestMachine) DrainAudio() aramcore.AudioChunk {
	panic("generating legacy drain must not be called")
}
func (m *publishedTestMachine) DrainPublishedAudio() aramcore.AudioChunk {
	if len(m.chunks) == 0 {
		return aramcore.AudioChunk{}
	}
	chunk := m.chunks[0]
	m.chunks = m.chunks[1:]
	return chunk
}

func TestAudioCollectionPrefersNonGeneratingPublishedDrain(t *testing.T) {
	machine := &publishedTestMachine{chunks: []aramcore.AudioChunk{{SampleRate: 44100, Channels: 1, PCM16: []int16{1, -1}}}}
	digest := sha256.New()
	result := framebench.Result{}
	if err := drainAudio(machine, digest, &result); err != nil {
		t.Fatal(err)
	}
	if result.AudioFrames != 2 || len(machine.chunks) != 0 {
		t.Fatal("published PCM was not consumed exactly once")
	}
	machine.chunks = []aramcore.AudioChunk{{Channels: 0, PCM16: []int16{1}}}
	if err := drainAudio(machine, digest, &result); err == nil {
		t.Fatal("invalid PCM channel layout accepted")
	}
}
