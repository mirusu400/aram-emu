package main

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/mirusu400/aram-core/application"
	"github.com/mirusu400/aram-core/cpu"
	"github.com/mirusu400/aram-emu/integration"
	"github.com/mirusu400/aram-frontend/frontend"
)

type performanceBackendStub struct {
	frames       uint64
	audioPending bool
}

func (backend *performanceBackendStub) RunFrame(context.Context) error {
	backend.frames++
	backend.audioPending = true
	return nil
}

func (backend *performanceBackendStub) VideoFrame() frontend.VideoFrame {
	return frontend.VideoFrame{Sequence: backend.frames}
}

func (backend *performanceBackendStub) DrainAudio() frontend.AudioChunk {
	if !backend.audioPending {
		return frontend.AudioChunk{}
	}
	backend.audioPending = false
	return frontend.AudioChunk{
		SampleRate:  44_100,
		Channels:    2,
		PCM16:       make([]int16, 4),
		StartSample: (backend.frames - 1) * 2,
		Generation:  1,
	}
}

func (backend *performanceBackendStub) Diagnostics() integration.Diagnostics {
	return integration.Diagnostics{
		State: frontend.StateRunning,
		Execution: &integration.ExecutionDiagnostics{
			Reason:       "budget",
			Instructions: 128,
		},
	}
}

func (backend *performanceBackendStub) CoreDebugSnapshot(
	int,
) (application.DebugSnapshot, bool) {
	return application.DebugSnapshot{
		Runtime: "stub",
		Execution: &cpu.ExecutionStatistics{
			FastContextSaves:         backend.frames,
			FastContextRestores:      backend.frames,
			TranslationInvalidations: backend.frames,
			TranslatedBlocks:         backend.frames * 2,
			TranslatedGuestBytes:     backend.frames * 4,
			TranslatedHostBytes:      backend.frames * 8,
			NativeArenaResets:        backend.frames,
			HostFrameCaptures:        backend.frames,
			HostRegisterCommits:      backend.frames,
		},
		Audio: &application.DebugAudioSnapshot{
			PublishedDropped:    backend.frames,
			MediaDroppedSamples: backend.frames * 2,
		},
	}, true
}

func TestSplitControlsTrimsAndDropsEmptyItems(t *testing.T) {
	if got := splitControls(" ok, , left ,,soft-right "); !slices.Equal(
		got,
		[]string{"ok", "left", "soft-right"},
	) {
		t.Fatalf("splitControls() = %q", got)
	}
}

func TestPerformanceFrameAccumulatorReportsTailStalls(t *testing.T) {
	var accumulator performanceFrameAccumulator
	for _, elapsed := range []time.Duration{
		time.Millisecond,
		2 * time.Millisecond,
		20 * time.Millisecond,
		40 * time.Millisecond,
		101 * time.Millisecond,
	} {
		accumulator.observe(elapsed)
	}
	result := accumulator.result()
	if result.P50US != 20_000 || result.P95US != 101_000 ||
		result.MaxUS != 101_000 || result.Over16_7MS != 3 ||
		result.Over33_3MS != 2 || result.Over100MS != 1 {
		t.Fatalf("frame timing = %+v", result)
	}
}

func TestPerformanceAudioDetectsGapsAndGenerationChanges(t *testing.T) {
	var accumulator performanceAudioAccumulator
	for _, chunk := range []frontend.AudioChunk{
		{SampleRate: 44_100, Channels: 2, PCM16: make([]int16, 4), StartSample: 0, Generation: 1},
		{SampleRate: 44_100, Channels: 2, PCM16: make([]int16, 4), StartSample: 2, Generation: 1},
		{SampleRate: 44_100, Channels: 2, PCM16: make([]int16, 4), StartSample: 5, Generation: 1},
		{SampleRate: 44_100, Channels: 2, PCM16: make([]int16, 4), StartSample: 0, Generation: 2},
	} {
		accumulator.observe(chunk)
	}
	result := accumulator.result
	if result.Chunks != 4 || result.Frames != 8 ||
		result.Discontinuities != 1 || result.MissingFrames != 1 ||
		result.GenerationChanges != 1 || result.InvalidChunks != 0 {
		t.Fatalf("audio continuity = %+v", result)
	}
}

func TestExecutionStatisticsDeltaIncludesHotPathCounters(t *testing.T) {
	before := cpu.ExecutionStatistics{
		FastContextSaves:         10,
		TranslationInvalidations: 3,
		TranslatedBlocks:         20,
		HostFrameCaptures:        40,
	}
	after := cpu.ExecutionStatistics{
		FastContextSaves:         14,
		TranslationInvalidations: 5,
		TranslatedBlocks:         29,
		HostFrameCaptures:        52,
	}
	delta := executionStatisticsDelta(&before, &after)
	if delta.FastContextSaves != 4 || delta.TranslationInvalidations != 2 ||
		delta.TranslatedBlocks != 9 || delta.HostFrameCaptures != 12 {
		t.Fatalf("execution delta = %+v", delta)
	}
}

func TestRunPerformancePublishesCompleteMeasurement(t *testing.T) {
	backend := &performanceBackendStub{}
	result := probeResult{State: frontend.StateRunning}
	err := runPerformance(
		context.Background(),
		backend,
		&result,
		performanceSettings{
			WarmupFrames: 1,
			Duration:     time.Nanosecond,
			CPU:          "native",
			AudioMode:    "faithful",
			TraceMode:    "counters",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	performance := result.Performance
	if performance == nil || performance.MeasuredFrames == 0 ||
		performance.Instructions == 0 || performance.FrameChanges == 0 ||
		performance.Audio.Chunks == 0 ||
		performance.Audio.PublishedDroppedSamples == 0 ||
		performance.Execution.FastContextSaves == 0 ||
		performance.Execution.HostRegisterCommits == 0 {
		t.Fatalf("performance measurement = %+v", performance)
	}
}

func TestClassifyErrorPreservesActionableCompatibilityStatus(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		format     string
		wantStatus string
		wantLevel  string
	}{
		{
			name: "unsupported format",
			err: &frontend.BackendError{
				Kind: frontend.FailureUnsupportedProfile,
				Err:  errors.New("unsupported"),
			},
			format:     "java-archive",
			wantStatus: "unsupported_format",
			wantLevel:  "recognized",
		},
		{
			name: "malformed recognized input",
			err: &frontend.BackendError{
				Kind: frontend.FailureMalformedInput,
				Err:  errors.New("bad input"),
			},
			format:     "wipi-dat",
			wantStatus: "malformed_input",
			wantLevel:  "recognized",
		},
		{
			name: "unsupported instruction",
			err: &frontend.BackendError{
				Kind: frontend.FailureGuestFaulted,
				Err:  cpu.ErrUnsupportedInstruction,
			},
			format:     "eads",
			wantStatus: "unimplemented_instruction",
			wantLevel:  "loads",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, level, _ := classifyError(test.err, test.format)
			if status != test.wantStatus || level != test.wantLevel {
				t.Fatalf("classifyError() = %q, %q; want %q, %q",
					status, level, test.wantStatus, test.wantLevel)
			}
		})
	}
}

func TestProbeDeadlineAfterGuestProgressIsAlive(t *testing.T) {
	result := probeResult{
		State:             frontend.StatePaused,
		TotalInstructions: 1,
	}
	if !probeDeadlineReachedAfterProgress(context.DeadlineExceeded, result) {
		t.Fatal("deadline after guest progress was not classified as alive")
	}

	result.TotalInstructions = 0
	if probeDeadlineReachedAfterProgress(context.DeadlineExceeded, result) {
		t.Fatal("deadline without guest progress was classified as alive")
	}

	result.TotalInstructions = 1
	result.State = frontend.StateFaulted
	if probeDeadlineReachedAfterProgress(context.DeadlineExceeded, result) {
		t.Fatal("faulted guest was classified as alive")
	}
}
