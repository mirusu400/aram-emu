//go:build js && wasm && aram_benchmark

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"os"
	"strings"
	"syscall/js"
	"time"

	"github.com/mirusu400/aram-emu/integration"
	"github.com/mirusu400/aram-emu/internal/framebench"
	"github.com/mirusu400/aram-frontend/frontend"
)

// Only a benchmark-tagged local build accepts this host-supplied scenario.
// Ordinary builds cannot replay controls or publish benchmark globals.
func configureBenchmark(backend *integration.Backend) frontend.Backend {
	global := js.Global()
	value := global.Get("__aramBenchmarkScenario")
	if value.Type() == js.TypeUndefined || value.IsNull() {
		return backend
	}
	host := global.Get("location").Get("hostname").String()
	if host != "127.0.0.1" && host != "localhost" && host != "[::1]" {
		panic("benchmarks require a loopback host")
	}
	scenario, err := framebench.ReadScenario(strings.NewReader(global.Get("JSON").Call("stringify", value).String()))
	if err != nil {
		panic("invalid benchmark scenario")
	}
	global.Delete("__aramBenchmarkScenario")
	cpu := "jit"
	if value := global.Get("__aramBenchmarkCPU"); value.Type() == js.TypeString {
		cpu = value.String()
	}
	if err := os.Setenv("ARAM_KTF_TRACE", "off"); err != nil {
		panic(err)
	}
	return &benchmarkBackend{Backend: backend, scenario: scenario, cpu: cpu}
}

type benchmarkBackend struct {
	*integration.Backend
	scenario  framebench.Scenario
	cpu       string
	session   *framebench.Session
	profileID string
}

func (b *benchmarkBackend) ConfigureCPU(frontend.CPUSettings) error {
	return b.Backend.ConfigureCPU(frontend.CPUSettings{Name: b.cpu})
}

func (b *benchmarkBackend) ConfigureAudio(settings frontend.AudioSettings) error {
	settings.MixMode, settings.OutputSampleRate, settings.OutputChannels = false, 44100, 1
	return b.Backend.ConfigureAudio(settings)
}

func (b *benchmarkBackend) ConfigureFont(frontend.FontSettings) error {
	return b.Backend.ConfigureFont(frontend.FontSettings{Name: "galmuri9"})
}

func (b *benchmarkBackend) ConfigureDisplay(frontend.DisplaySettings) error {
	return b.Backend.ConfigureDisplay(frontend.DisplaySettings{})
}

func (b *benchmarkBackend) OpenWithProgress(ctx context.Context, request frontend.OpenRequest, progress func(frontend.OpenStage)) (frontend.InputInfo, error) {
	if b.session != nil {
		return frontend.InputInfo{}, fmt.Errorf("reload the benchmark page before opening another input")
	}
	info, err := b.Backend.OpenWithProgress(ctx, request, progress)
	if err != nil {
		publishBenchmark(framebench.Result{Schema: 1, Status: "failed", Error: "product open failed"})
		return info, err
	}
	if info.SHA256 != b.scenario.SHA256 {
		publishBenchmark(framebench.Result{Schema: 1, Status: "failed", Error: "input identity mismatch"})
		return info, fmt.Errorf("benchmark input identity mismatch")
	}
	b.profileID = info.ProfileID
	return info, nil
}

func (b *benchmarkBackend) Open(ctx context.Context, request frontend.OpenRequest) (frontend.InputInfo, error) {
	return b.OpenWithProgress(ctx, request, nil)
}

func (b *benchmarkBackend) QueueInput(frontend.InputEvent) error {
	result := framebench.Result{Schema: 1, Status: "failed", Error: "live input invalidates replay"}
	if b.session != nil {
		_ = b.session.Fail(result.Error)
		result = b.session.Result
	}
	publishBenchmark(result)
	return fmt.Errorf("%s", result.Error)
}

func (b *benchmarkBackend) RunFrame(ctx context.Context) error {
	if b.session == nil {
		session, err := framebench.New(b.scenario, "browser-product-paced", b.cpu, b.FrameQuantum())
		if err != nil {
			return err
		}
		b.session = session
		session.Result.QuantumSource = "product-scheduled"
		session.Result.ProfileID = b.profileID
		if snapshot, ok := b.CoreDebugSnapshot(1); ok {
			session.Result.Runtime = snapshot.Runtime
			if snapshot.CPU != nil {
				session.Result.ResolvedCPU = snapshot.CPU.Name
			}
		}
		js.Global().Set("__aramBenchmarkRunning", true)
	}
	session := b.session
	if session.Done() {
		return nil
	}
	// A worker may still owe frames after the guest stops. Those adapter no-ops
	// must never become fake fast benchmark samples.
	if b.Backend.State() != frontend.StateRunning {
		err := session.Fail("product not running before scenario completed")
		publishBenchmark(session.Result)
		return err
	}
	if session.Result.CompletedFrames == b.scenario.WarmupFrames {
		js.Global().Set("__aramBenchmarkMeasuring", true)
	}
	if err := session.BeforeFrame(time.Now(), func(event framebench.Event) error {
		return b.Backend.QueueInput(frontend.InputEvent{Control: event.Control, Pressed: event.Pressed})
	}); err != nil {
		publishBenchmark(session.Result)
		return err
	}
	started := time.Now()
	if err := b.Backend.RunFrame(ctx); err != nil {
		_ = session.Fail("guest frame failed")
		publishBenchmark(session.Result)
		return err
	}
	elapsed := time.Since(started)
	if err := session.SetStepQuantum(b.FrameQuantum()); err != nil {
		publishBenchmark(session.Result)
		return err
	}
	var frame image.Image
	if session.NeedsCheckpoint() {
		frame = b.VideoFrame().Image
	}
	if err := session.AfterFrame(time.Now(), elapsed, frame); err != nil {
		publishBenchmark(session.Result)
		return err
	}
	if session.Result.CompletedFrames%b.scenario.SegmentFrames == 0 {
		js.Global().Set("__aramBenchmarkProgress", float64(session.Result.CompletedFrames))
	}
	if session.Done() {
		// Pause through the ordinary adapter, leaving the final screen visible.
		if err := b.Backend.Execute(ctx, frontend.CommandPauseResume); err != nil {
			_ = session.Fail("final pause failed")
		}
		publishBenchmark(session.Result)
	}
	return nil
}

func publishBenchmark(result framebench.Result) {
	encoded, err := json.Marshal(result)
	if err != nil {
		panic(err)
	}
	global := js.Global()
	global.Set("__aramBenchmarkResult", global.Get("JSON").Call("parse", string(encoded)))
	global.Set("__aramBenchmarkRunning", false)
}
