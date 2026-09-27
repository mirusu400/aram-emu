// aram-core-bench measures fixed guest-frame scenarios without host pacing.
// It uses the product's portable factory defaults, but deliberately excludes
// the frontend, rendering and output audio device. It is not a playability probe.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"hash"
	"image"
	"io"
	"os"
	"path/filepath"
	"runtime/pprof"
	"time"

	"github.com/mirusu400/aram-core/application"
	aramcore "github.com/mirusu400/aram-core/core"
	"github.com/mirusu400/aram-emu/internal/framebench"
	"github.com/mirusu400/aram-emu/internal/productconfig"
)

func main() { os.Exit(run()) }

func run() int {
	input := flag.String("input", "", "operator-authorized input, read in place")
	inputURL := flag.String("input-url", "", "optional loopback input stream (e.g. adb reverse); never stored on device")
	scenarioPath := flag.String("scenario", "", "exact-hash frame scenario JSON")
	scenarioURL := flag.String("scenario-url", "", "optional loopback scenario stream")
	cpu := flag.String("cpu", "jit", "precise, jit, native or fastest")
	timeout := flag.Duration("timeout", 5*time.Minute, "whole run timeout")
	profile := flag.String("cpuprofile", "", "optional native CPU profile; do not compare profiled timings")
	flag.Parse()
	failure := func(message string) int {
		_ = json.NewEncoder(os.Stdout).Encode(framebench.Result{Schema: 1, Kind: "core-unpaced", Status: "failed", Error: message})
		return 1
	}
	if (*input == "") == (*inputURL == "") || (*scenarioPath == "") == (*scenarioURL == "") || *timeout <= 0 {
		return failure("one input source, one scenario source and positive timeout required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	scenarioFile, err := openBenchmarkSource(ctx, *scenarioPath, *scenarioURL)
	if err != nil {
		return failure("scenario unavailable")
	}
	scenario, err := framebench.ReadScenario(scenarioFile)
	_ = scenarioFile.Close()
	if err != nil {
		return failure("invalid scenario")
	}
	file, err := openBenchmarkSource(ctx, *input, *inputURL)
	if err != nil {
		return failure("authorized input unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(file, (512<<20)+1))
	_ = file.Close()
	if err != nil || len(data) == 0 || len(data) > 512<<20 {
		return failure("invalid authorized input size")
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != scenario.SHA256 {
		return failure("input identity mismatch")
	}
	if err := os.Setenv("ARAM_KTF_TRACE", "off"); err != nil {
		return failure("trace configuration failed")
	}
	factory := productconfig.ApplicationFactory()
	factory.NewCPU, err = application.ResolveCPUBackend(*cpu)
	if err != nil {
		return failure("CPU backend unavailable")
	}
	sourceName := filepath.Base(*input)
	if *inputURL != "" {
		// Preserve a privacy-safe package label, never a host filesystem name.
		sourceName = scenario.ID + ".zip"
	}
	machine, err := factory.Create(ctx, aramcore.Source{Name: sourceName, ReaderAt: bytes.NewReader(data), Size: int64(len(data)), SHA256: scenario.SHA256})
	if err != nil {
		return failure("machine creation failed")
	}
	defer machine.Close()
	if err := machine.Start(ctx); err != nil {
		return failure("machine start failed")
	}
	reporter, reportedQuantum := machine.(interface{ FrameQuantum() time.Duration })
	quantum := 16 * time.Millisecond // Same explicitly labelled product fallback.
	if reportedQuantum {
		quantum = reporter.FrameQuantum()
	}
	session, err := framebench.New(scenario, "core-unpaced", *cpu, quantum)
	if err != nil {
		return failure("benchmark initialization failed")
	}
	session.Result.QuantumSource = "product-fallback-estimate"
	if reportedQuantum {
		session.Result.QuantumSource = "runtime-reported"
	}
	session.Result.AudioDrain = "legacy-drain"
	session.Result.MachineImplementation = fmt.Sprintf("%T", machine)
	if provider, ok := machine.(interface{ ImageInfo() application.ImageInfo }); ok {
		session.Result.ProfileID = provider.ImageInfo().ProfileID
	}
	if _, ok := machine.(interface{ DrainPublishedAudio() aramcore.AudioChunk }); ok {
		session.Result.AudioDrain = "published-non-generating"
	}
	if snapshotter, ok := machine.(interface {
		DebugSnapshot(int) application.DebugSnapshot
	}); ok {
		snapshot := snapshotter.DebugSnapshot(1)
		session.Result.Runtime = snapshot.Runtime
		if snapshot.CPU != nil {
			session.Result.ResolvedCPU = snapshot.CPU.Name
		}
	}
	if *profile != "" {
		file, err := os.Create(*profile)
		if err != nil {
			return failure("CPU profile unavailable")
		}
		defer file.Close()
		if err := pprof.StartCPUProfile(file); err != nil {
			return failure("CPU profiling unavailable")
		}
		defer pprof.StopCPUProfile()
	}
	audio := sha256.New()
	for !session.Done() {
		if !canContinue(machine.State()) {
			_ = session.Fail("guest stopped before scenario completed")
			break
		}
		if err := ctx.Err(); err != nil {
			_ = session.Fail("benchmark timeout")
			break
		}
		if err := session.BeforeFrame(time.Now(), func(event framebench.Event) error {
			return machine.QueueInput(aramcore.InputEvent{Control: event.Control, Pressed: event.Pressed})
		}); err != nil {
			break
		}
		started := time.Now()
		if err := machine.StepFrame(ctx); err != nil {
			_ = session.Fail("guest frame failed")
			break
		}
		if err := drainAudio(machine, audio, &session.Result); err != nil {
			_ = session.Fail(err.Error())
			break
		}
		elapsed := time.Since(started)
		if reportedQuantum {
			if err := session.SetStepQuantum(reporter.FrameQuantum()); err != nil {
				break
			}
		}
		var frame image.Image
		if session.NeedsCheckpoint() {
			frame = machine.Framebuffer()
		}
		if err := session.AfterFrame(time.Now(), elapsed, frame); err != nil {
			break
		}
	}
	session.Result.AudioSHA256 = hex.EncodeToString(audio.Sum(nil))
	_ = json.NewEncoder(os.Stdout).Encode(session.Result)
	if session.Result.Status != "complete" {
		return 1
	}
	return 0
}

func canContinue(state aramcore.State) bool {
	return state == aramcore.StateRunning || state == aramcore.StatePaused
}

func drainAudio(machine aramcore.Machine, digest hash.Hash, result *framebench.Result) error {
	drain := machine.DrainAudio
	if published, ok := machine.(interface{ DrainPublishedAudio() aramcore.AudioChunk }); ok {
		drain = published.DrainPublishedAudio
	}
	for range 4096 {
		chunk := drain()
		if len(chunk.PCM16) == 0 {
			return nil
		}
		if chunk.Channels <= 0 || len(chunk.PCM16)%chunk.Channels != 0 {
			return fmt.Errorf("invalid guest audio chunk")
		}
		var header [40]byte
		binary.LittleEndian.PutUint64(header[0:], uint64(chunk.StartGuestNS))
		binary.LittleEndian.PutUint64(header[8:], chunk.StartSample)
		binary.LittleEndian.PutUint64(header[16:], chunk.Generation)
		binary.LittleEndian.PutUint32(header[24:], uint32(chunk.SampleRate))
		binary.LittleEndian.PutUint32(header[28:], uint32(chunk.Channels))
		binary.LittleEndian.PutUint64(header[32:], uint64(len(chunk.PCM16)))
		_, _ = digest.Write(header[:])
		pcm := make([]byte, 2*len(chunk.PCM16))
		for index, sample := range chunk.PCM16 {
			binary.LittleEndian.PutUint16(pcm[2*index:], uint16(sample))
		}
		_, _ = digest.Write(pcm)
		result.AudioFrames += uint64(len(chunk.PCM16) / chunk.Channels)
	}
	return fmt.Errorf("audio drain limit exceeded")
}
