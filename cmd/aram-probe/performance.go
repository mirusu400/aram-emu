package main

import (
	"context"
	"errors"
	"time"

	"github.com/mirusu400/aram-core/application"
	"github.com/mirusu400/aram-core/cpu"
	"github.com/mirusu400/aram-emu/integration"
	"github.com/mirusu400/aram-frontend/frontend"
)

const (
	performanceFrameBucketWidth = 100 * time.Microsecond
	performanceFrameBuckets     = 2001
)

type performanceSettings struct {
	WarmupFrames uint64
	Duration     time.Duration
	CPU          string
	AudioMode    string
	TraceMode    string
}

type performanceBackend interface {
	RunFrame(context.Context) error
	VideoFrame() frontend.VideoFrame
	DrainAudio() frontend.AudioChunk
	Diagnostics() integration.Diagnostics
	CoreDebugSnapshot(int) (application.DebugSnapshot, bool)
}

type performanceResult struct {
	CPU                   string                  `json:"cpu"`
	AudioMode             string                  `json:"audio_mode"`
	TraceMode             string                  `json:"trace_mode"`
	Runtime               string                  `json:"runtime,omitempty"`
	WarmupFrames          uint64                  `json:"warmup_frames"`
	MeasuredFrames        uint64                  `json:"measured_frames"`
	MeasurementMS         int64                   `json:"measurement_ms"`
	ActiveFrameMS         float64                 `json:"active_frame_ms"`
	FramesPerSecond       float64                 `json:"frames_per_second"`
	WallFramesPerSecond   float64                 `json:"wall_frames_per_second"`
	Instructions          uint64                  `json:"instructions"`
	InstructionsPerSecond float64                 `json:"instructions_per_second"`
	FrameChanges          uint64                  `json:"frame_changes"`
	FrameTime             performanceFrameTime    `json:"frame_time"`
	Audio                 performanceAudio        `json:"audio"`
	Execution             cpu.ExecutionStatistics `json:"execution"`
	KTF                   *performanceKTF         `json:"ktf,omitempty"`
}

type performanceFrameTime struct {
	MeanUS     float64 `json:"mean_us"`
	P50US      int64   `json:"p50_us"`
	P95US      int64   `json:"p95_us"`
	P99US      int64   `json:"p99_us"`
	MaxUS      int64   `json:"max_us"`
	Over16_7MS uint64  `json:"over_16_7_ms"`
	Over33_3MS uint64  `json:"over_33_3_ms"`
	Over100MS  uint64  `json:"over_100_ms"`
}

type performanceAudio struct {
	Chunks                  uint64 `json:"chunks"`
	Frames                  uint64 `json:"frames"`
	GenerationChanges       uint64 `json:"generation_changes"`
	Discontinuities         uint64 `json:"discontinuities"`
	MissingFrames           uint64 `json:"missing_frames"`
	OverlappingFrames       uint64 `json:"overlapping_frames"`
	FormatChanges           uint64 `json:"format_changes"`
	InvalidChunks           uint64 `json:"invalid_chunks"`
	SampleRate              int    `json:"sample_rate,omitempty"`
	Channels                int    `json:"channels,omitempty"`
	PublishedDroppedSamples uint64 `json:"published_dropped_samples"`
	MediaDroppedSamples     uint64 `json:"media_dropped_samples"`
}

type performanceKTF struct {
	Presentations    uint64 `json:"presentations"`
	TaskInstructions uint64 `json:"task_instructions"`
	TaskSlices       uint64 `json:"task_slices"`
	TaskYields       uint64 `json:"task_yields"`
	HostCalls        uint64 `json:"host_calls"`
}

type performanceFrameAccumulator struct {
	buckets    [performanceFrameBuckets]uint64
	count      uint64
	total      time.Duration
	maximum    time.Duration
	over16_7MS uint64
	over33_3MS uint64
	over100MS  uint64
}

func (a *performanceFrameAccumulator) observe(elapsed time.Duration) {
	if elapsed < 0 {
		elapsed = 0
	}
	a.count++
	a.total += elapsed
	if elapsed > a.maximum {
		a.maximum = elapsed
	}
	if elapsed > time.Second/60 {
		a.over16_7MS++
	}
	if elapsed > time.Second/30 {
		a.over33_3MS++
	}
	if elapsed > 100*time.Millisecond {
		a.over100MS++
	}
	bucket := int(elapsed / performanceFrameBucketWidth)
	if bucket >= len(a.buckets) {
		bucket = len(a.buckets) - 1
	}
	a.buckets[bucket]++
}

func (a *performanceFrameAccumulator) result() performanceFrameTime {
	result := performanceFrameTime{
		MaxUS:      a.maximum.Microseconds(),
		Over16_7MS: a.over16_7MS,
		Over33_3MS: a.over33_3MS,
		Over100MS:  a.over100MS,
	}
	if a.count == 0 {
		return result
	}
	result.MeanUS = float64(a.total.Microseconds()) / float64(a.count)
	result.P50US = a.quantileUS(50, 100)
	result.P95US = a.quantileUS(95, 100)
	result.P99US = a.quantileUS(99, 100)
	return result
}

func (a *performanceFrameAccumulator) quantileUS(numerator, denominator uint64) int64 {
	if a.count == 0 || denominator == 0 {
		return 0
	}
	target := (a.count*numerator + denominator - 1) / denominator
	var seen uint64
	for index, count := range a.buckets {
		seen += count
		if seen >= target {
			return int64(index) * performanceFrameBucketWidth.Microseconds()
		}
	}
	return a.maximum.Microseconds()
}

type performanceAudioAccumulator struct {
	result         performanceAudio
	haveChunk      bool
	generation     uint64
	expectedStart  uint64
	lastSampleRate int
	lastChannels   int
}

func (a *performanceAudioAccumulator) observe(chunk frontend.AudioChunk) {
	if chunk.SampleRate <= 0 || chunk.Channels <= 0 || len(chunk.PCM16) == 0 ||
		len(chunk.PCM16)%chunk.Channels != 0 {
		if len(chunk.PCM16) != 0 {
			a.result.InvalidChunks++
		}
		return
	}
	frames := uint64(len(chunk.PCM16) / chunk.Channels)
	if a.haveChunk && (chunk.SampleRate != a.lastSampleRate ||
		chunk.Channels != a.lastChannels) {
		a.result.FormatChanges++
	}
	if !a.haveChunk || chunk.Generation != a.generation {
		if a.haveChunk {
			a.result.GenerationChanges++
		}
		a.generation = chunk.Generation
		a.expectedStart = chunk.StartSample
	} else if chunk.StartSample != a.expectedStart {
		a.result.Discontinuities++
		if chunk.StartSample > a.expectedStart {
			a.result.MissingFrames += chunk.StartSample - a.expectedStart
		} else {
			a.result.OverlappingFrames += a.expectedStart - chunk.StartSample
		}
	}
	a.expectedStart = chunk.StartSample + frames
	a.lastSampleRate = chunk.SampleRate
	a.lastChannels = chunk.Channels
	a.haveChunk = true
	a.result.Chunks++
	a.result.Frames += frames
	a.result.SampleRate = chunk.SampleRate
	a.result.Channels = chunk.Channels
}

func drainPerformanceAudio(
	backend performanceBackend,
	measurement *performanceAudioAccumulator,
) {
	for range 4096 {
		chunk := backend.DrainAudio()
		if len(chunk.PCM16) == 0 {
			return
		}
		if measurement != nil {
			measurement.observe(chunk)
		}
	}
}

func runPerformance(
	ctx context.Context,
	backend performanceBackend,
	result *probeResult,
	settings performanceSettings,
) error {
	for range settings.WarmupFrames {
		if result.State != frontend.StateRunning {
			return errors.New("machine stopped during performance warmup")
		}
		if err := backend.RunFrame(ctx); err != nil {
			return err
		}
		_ = backend.VideoFrame()
		drainPerformanceAudio(backend, nil)
		updatePerformanceProgress(result, backend.Diagnostics())
	}

	before, _ := backend.CoreDebugSnapshot(1)
	instructionsBefore := result.TotalInstructions
	frameBefore := backend.VideoFrame().Sequence
	measurement := performanceAudioAccumulator{}
	timing := performanceFrameAccumulator{}
	started := time.Now()
	var active time.Duration
	for timing.count == 0 || time.Since(started) < settings.Duration {
		if result.State != frontend.StateRunning {
			return errors.New("machine stopped during performance measurement")
		}
		frameStarted := time.Now()
		err := backend.RunFrame(ctx)
		_ = backend.VideoFrame()
		drainPerformanceAudio(backend, &measurement)
		frameElapsed := time.Since(frameStarted)
		timing.observe(frameElapsed)
		active += frameElapsed
		updatePerformanceProgress(result, backend.Diagnostics())
		if err != nil {
			return err
		}
	}
	wall := time.Since(started)
	after, _ := backend.CoreDebugSnapshot(1)
	copyDiagnostics(result, backend.Diagnostics())
	frameAfter := backend.VideoFrame().Sequence

	measuredInstructions := deltaCounter(instructionsBefore, result.TotalInstructions)
	performance := &performanceResult{
		CPU:            settings.CPU,
		AudioMode:      settings.AudioMode,
		TraceMode:      settings.TraceMode,
		Runtime:        after.Runtime,
		WarmupFrames:   settings.WarmupFrames,
		MeasuredFrames: timing.count,
		MeasurementMS:  wall.Milliseconds(),
		ActiveFrameMS:  float64(active.Microseconds()) / 1000,
		Instructions:   measuredInstructions,
		FrameChanges:   deltaCounter(frameBefore, frameAfter),
		FrameTime:      timing.result(),
		Audio:          measurement.result,
		Execution:      executionStatisticsDelta(before.Execution, after.Execution),
	}
	if result.Image != nil && result.Image.CPUBackend != "" {
		performance.CPU = result.Image.CPUBackend
	}
	if active > 0 {
		seconds := active.Seconds()
		performance.FramesPerSecond = float64(timing.count) / seconds
		performance.InstructionsPerSecond = float64(measuredInstructions) / seconds
	}
	if wall > 0 {
		performance.WallFramesPerSecond = float64(timing.count) / wall.Seconds()
	}
	if before.Audio != nil && after.Audio != nil {
		performance.Audio.PublishedDroppedSamples = deltaCounter(
			before.Audio.PublishedDropped,
			after.Audio.PublishedDropped,
		)
		performance.Audio.MediaDroppedSamples = deltaCounter(
			before.Audio.MediaDroppedSamples,
			after.Audio.MediaDroppedSamples,
		)
	}
	if before.KTF != nil && after.KTF != nil {
		performance.TraceMode = after.KTF.TraceMode
		performance.KTF = &performanceKTF{
			Presentations: deltaCounter(
				uint64(before.KTF.PresentCount),
				uint64(after.KTF.PresentCount),
			),
			TaskInstructions: deltaCounter(
				before.KTF.TaskInstructions,
				after.KTF.TaskInstructions,
			),
			TaskSlices: deltaCounter(before.KTF.TaskSlices, after.KTF.TaskSlices),
			TaskYields: deltaCounter(before.KTF.TaskYields, after.KTF.TaskYields),
			HostCalls:  deltaCounter(before.KTF.HostCalls, after.KTF.HostCalls),
		}
	}
	result.Performance = performance
	return nil
}

func updatePerformanceProgress(result *probeResult, diagnostics integration.Diagnostics) {
	result.State = diagnostics.State
	if diagnostics.EADS != nil {
		result.TotalInstructions = diagnostics.EADS.TotalInstructions
	} else if diagnostics.Execution != nil {
		result.TotalInstructions += diagnostics.Execution.Instructions
	}
}

func executionStatisticsDelta(
	before, after *cpu.ExecutionStatistics,
) cpu.ExecutionStatistics {
	if before == nil || after == nil {
		return cpu.ExecutionStatistics{}
	}
	return cpu.ExecutionStatistics{
		SerializedContextSaves: deltaCounter(
			before.SerializedContextSaves, after.SerializedContextSaves,
		),
		SerializedContextRestores: deltaCounter(
			before.SerializedContextRestores, after.SerializedContextRestores,
		),
		FastContextSaves: deltaCounter(before.FastContextSaves, after.FastContextSaves),
		FastContextRestores: deltaCounter(
			before.FastContextRestores, after.FastContextRestores,
		),
		TranslationInvalidations: deltaCounter(
			before.TranslationInvalidations, after.TranslationInvalidations,
		),
		TranslatedBlocks: deltaCounter(before.TranslatedBlocks, after.TranslatedBlocks),
		TranslatedGuestBytes: deltaCounter(
			before.TranslatedGuestBytes, after.TranslatedGuestBytes,
		),
		TranslatedHostBytes: deltaCounter(
			before.TranslatedHostBytes, after.TranslatedHostBytes,
		),
		NativeArenaResets: deltaCounter(before.NativeArenaResets, after.NativeArenaResets),
		HostFrameCaptures: deltaCounter(
			before.HostFrameCaptures, after.HostFrameCaptures,
		),
		HostRegisterCommits: deltaCounter(
			before.HostRegisterCommits, after.HostRegisterCommits,
		),
	}
}

func deltaCounter(before, after uint64) uint64 {
	if after < before {
		return 0
	}
	return after - before
}
