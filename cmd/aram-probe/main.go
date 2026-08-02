// aram-probe runs one input through the ordinary headless ARAM product path
// and emits exactly one machine-readable compatibility record.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mirusu400/aram-core/cpu"
	"github.com/mirusu400/aram-emu/integration"
	"github.com/mirusu400/aram-frontend/frontend"
)

const resultSchema = 1

type probeResult struct {
	Schema            int                   `json:"schema"`
	Name              string                `json:"name"`
	Size              int64                 `json:"size,omitempty"`
	SHA256            string                `json:"sha256,omitempty"`
	Format            string                `json:"format,omitempty"`
	ProfileID         string                `json:"profile_id,omitempty"`
	Status            string                `json:"status"`
	Level             string                `json:"level,omitempty"`
	State             frontend.BackendState `json:"state"`
	OpenStages        []frontend.OpenStage  `json:"open_stages,omitempty"`
	Image             *imageResult          `json:"image,omitempty"`
	LastExecution     *executionResult      `json:"last_execution,omitempty"`
	WIPI              *wipiResult           `json:"wipi,omitempty"`
	EADS              *eadsResult           `json:"eads,omitempty"`
	TotalInstructions uint64                `json:"total_instructions,omitempty"`
	FirstFrameSlice   uint64                `json:"first_frame_slice,omitempty"`
	PreInputSlices    uint64                `json:"pre_input_slices,omitempty"`
	PostFrameSlices   uint64                `json:"post_frame_slices,omitempty"`
	InputEvents       uint64                `json:"input_events,omitempty"`
	FrameChanges      uint64                `json:"frame_changes,omitempty"`
	ErrorKind         frontend.FailureKind  `json:"error_kind,omitempty"`
	Detail            string                `json:"detail,omitempty"`
	ElapsedMS         int64                 `json:"elapsed_ms"`
}

type imageResult struct {
	Name       string `json:"name"`
	EntryPoint string `json:"entry_point"`
	Mode       string `json:"mode"`
}

type executionResult struct {
	Reason       string `json:"reason"`
	Instructions uint64 `json:"instructions"`
	PC           string `json:"pc"`
	Error        string `json:"error,omitempty"`
}

type eadsResult struct {
	Events            []eadsEventResult `json:"events"`
	PresentCount      uint32            `json:"present_count"`
	TickMS            uint32            `json:"tick_ms"`
	TotalInstructions uint64            `json:"total_instructions"`
	TotalAPICalls     uint64            `json:"total_api_calls"`
}

type wipiResult struct {
	PresentCount        uint32   `json:"present_count"`
	APICalls            uint64   `json:"api_calls"`
	ImplementedCalls    uint64   `json:"implemented_calls"`
	UnimplementedCalls  uint64   `json:"unimplemented_calls"`
	LastAPI             string   `json:"last_api,omitempty"`
	LastUnimplemented   string   `json:"last_unimplemented,omitempty"`
	CatalogedAPIs       int      `json:"cataloged_apis"`
	DispatchWiredAPIs   int      `json:"dispatch_wired_apis"`
	SemanticallyModeled int      `json:"semantically_modeled"`
	ObservedAPIs        int      `json:"observed_apis"`
	UnimplementedAPIs   []string `json:"unimplemented_apis,omitempty"`
}

type eadsEventResult struct {
	Event        string `json:"event"`
	Instructions uint64 `json:"instructions"`
	APICalls     uint64 `json:"api_calls"`
	ReturnValue  uint32 `json:"return_value"`
}

func main() {
	os.Exit(run())
}

func run() int {
	input := flag.String("input", "", "path to an authorized WIPI input")
	label := flag.String("label", "", "privacy-safe display name for the result")
	slices := flag.Uint64("slices", 256, "maximum one-instruction execution slices")
	postFrameSlices := flag.Uint64(
		"post-frame-slices",
		0,
		"execution slices to run after the first presented frame",
	)
	controls := flag.String(
		"controls",
		"",
		"comma-separated controls to press and release after the first frame",
	)
	preInputSlices := flag.Uint64(
		"pre-input-slices",
		0,
		"execution slices to run before sending the first control",
	)
	controlHoldSlices := flag.Uint64(
		"control-hold-slices",
		64,
		"execution slices between each control press and release",
	)
	controlReleaseSlices := flag.Uint64(
		"control-release-slices",
		4,
		"execution slices after each control release",
	)
	timeout := flag.Duration("timeout", 10*time.Second, "whole-probe timeout")
	flag.Parse()

	started := time.Now()
	result := probeResult{
		Schema: resultSchema,
		Status: "runner_error",
		State:  frontend.StateEmpty,
	}
	parsedControls := splitControls(*controls)
	if *input == "" || *slices == 0 || *timeout <= 0 ||
		len(parsedControls) != 0 &&
			(*controlHoldSlices == 0 || *controlReleaseSlices == 0) {
		result.Detail = "input, positive slices/timeout, and positive control slice counts are required"
		result.ElapsedMS = time.Since(started).Milliseconds()
		writeResult(result)
		return 2
	}
	result.Name = *label
	if result.Name == "" {
		result.Name = filepath.Base(*input)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	backend := integration.NewBackend(nil)
	defer backend.Close()

	var stages []frontend.OpenStage
	info, err := backend.OpenWithProgress(
		ctx,
		frontend.OpenRequest{
			Path:        *input,
			DisplayName: result.Name,
		},
		func(stage frontend.OpenStage) {
			stages = append(stages, stage)
		},
	)
	result.OpenStages = stages
	result.Size = info.Size
	result.SHA256 = info.SHA256
	result.Format = info.Format
	result.ProfileID = info.ProfileID
	result.State = backend.State()
	if err != nil {
		result.Status, result.Level, result.ErrorKind = classifyError(err, info.Format)
		result.Detail = err.Error()
		result.ElapsedMS = time.Since(started).Milliseconds()
		writeResult(result)
		return 1
	}

	result.Level = "loads"
	for slice := range *slices {
		err = runProbeSlice(ctx, backend, &result, slice == 0)
		diagnostics := backend.Diagnostics()
		if err != nil {
			if probeDeadlineReachedAfterProgress(err, result) {
				result.Status = "ok_alive"
				result.Detail = ""
				break
			}
			result.Status, _, result.ErrorKind = classifyError(err, result.Format)
			result.Detail = err.Error()
			break
		}
		if (diagnostics.EADS != nil && diagnostics.EADS.PresentCount > 0) ||
			(diagnostics.WIPI != nil && diagnostics.WIPI.PresentCount > 0) {
			result.Status = "ok_frame"
			result.Level = "boots"
			result.FirstFrameSlice = slice + 1
			break
		}
		if diagnostics.Execution != nil &&
			diagnostics.Execution.Reason == "breakpoint" {
			result.Status = "ok_service_boundary"
			break
		}
		if diagnostics.Execution != nil &&
			diagnostics.Execution.Reason == "exited" {
			result.Status = "ok_exit"
			break
		}
	}
	if result.Status == "ok_frame" &&
		(*postFrameSlices != 0 || len(parsedControls) != 0) {
		firstFrameSequence := backend.VideoFrame().Sequence
		if len(parsedControls) != 0 && *preInputSlices != 0 {
			before := result.PostFrameSlices
			err = runProbeFrames(
				ctx,
				backend,
				&result,
				*preInputSlices,
			)
			result.PreInputSlices = result.PostFrameSlices - before
			if err != nil {
				result.Status, _, result.ErrorKind = classifyError(err, result.Format)
				result.Detail = err.Error()
			}
		}
		for _, control := range parsedControls {
			if err != nil || result.State != frontend.StateRunning {
				break
			}
			err = backend.QueueInput(frontend.InputEvent{
				Control: control,
				Pressed: true,
			})
			if err != nil {
				result.Status, _, result.ErrorKind = classifyError(err, result.Format)
				result.Detail = err.Error()
				break
			}
			result.InputEvents++
			if err = runProbeFrames(
				ctx,
				backend,
				&result,
				*controlHoldSlices,
			); err != nil {
				break
			}
			if result.State != frontend.StateRunning {
				break
			}
			err = backend.QueueInput(frontend.InputEvent{
				Control: control,
				Pressed: false,
			})
			if err != nil {
				result.Status, _, result.ErrorKind = classifyError(err, result.Format)
				result.Detail = err.Error()
				break
			}
			result.InputEvents++
			if err = runProbeFrames(
				ctx,
				backend,
				&result,
				*controlReleaseSlices,
			); err != nil {
				break
			}
		}
		if err == nil && result.State == frontend.StateRunning {
			err = runProbeFrames(
				ctx,
				backend,
				&result,
				*postFrameSlices,
			)
		}
		if err != nil {
			result.Status, _, result.ErrorKind = classifyError(err, result.Format)
			result.Detail = err.Error()
		} else if result.State == frontend.StateRunning {
			if result.InputEvents != 0 {
				result.Level = "interactive"
			} else if result.PostFrameSlices != 0 {
				result.Level = "sustained"
			}
		}
		lastFrameSequence := backend.VideoFrame().Sequence
		if lastFrameSequence > firstFrameSequence {
			result.FrameChanges = lastFrameSequence - firstFrameSequence
		}
	}
	if result.Status == "runner_error" {
		if err := ctx.Err(); err != nil {
			result.Status = "timeout"
			result.Detail = err.Error()
		} else {
			result.Status = "ok_alive"
		}
	}
	result.ElapsedMS = time.Since(started).Milliseconds()
	writeResult(result)
	if result.Status == "ok_frame" ||
		result.Status == "ok_alive" ||
		result.Status == "ok_service_boundary" ||
		result.Status == "ok_exit" {
		return 0
	}
	return 1
}

func splitControls(value string) []string {
	var controls []string
	for _, control := range strings.Split(value, ",") {
		if control = strings.TrimSpace(control); control != "" {
			controls = append(controls, control)
		}
	}
	return controls
}

func probeDeadlineReachedAfterProgress(err error, result probeResult) bool {
	if !errors.Is(err, context.DeadlineExceeded) ||
		result.TotalInstructions == 0 {
		return false
	}
	return result.State == frontend.StateRunning ||
		result.State == frontend.StatePaused
}

func runProbeSlice(
	ctx context.Context,
	backend *integration.Backend,
	result *probeResult,
	start bool,
) error {
	var err error
	if start {
		err = backend.Execute(ctx, frontend.CommandStart)
	} else {
		err = backend.RunFrame(ctx)
	}
	diagnostics := backend.Diagnostics()
	result.State = diagnostics.State
	copyDiagnostics(result, diagnostics)
	if diagnostics.EADS != nil {
		result.TotalInstructions = diagnostics.EADS.TotalInstructions
	} else if diagnostics.Execution != nil {
		result.TotalInstructions += diagnostics.Execution.Instructions
	}
	return err
}

func runProbeFrames(
	ctx context.Context,
	backend *integration.Backend,
	result *probeResult,
	count uint64,
) error {
	for range count {
		if result.State != frontend.StateRunning {
			return nil
		}
		if err := runProbeSlice(ctx, backend, result, false); err != nil {
			return err
		}
		result.PostFrameSlices++
		_ = backend.VideoFrame()
	}
	return nil
}

func copyDiagnostics(result *probeResult, diagnostics integration.Diagnostics) {
	if diagnostics.Image != nil {
		result.Image = &imageResult{
			Name:       diagnostics.Image.Name,
			EntryPoint: fmt.Sprintf("0x%08x", diagnostics.Image.EntryPoint),
			Mode:       diagnostics.Image.Mode,
		}
	}
	if diagnostics.Execution != nil {
		result.LastExecution = &executionResult{
			Reason:       diagnostics.Execution.Reason,
			Instructions: diagnostics.Execution.Instructions,
			PC:           fmt.Sprintf("0x%08x", diagnostics.Execution.PC),
			Error:        diagnostics.Execution.Error,
		}
	}
	if diagnostics.EADS != nil {
		result.EADS = &eadsResult{
			PresentCount:      diagnostics.EADS.PresentCount,
			TickMS:            diagnostics.EADS.TickMS,
			TotalInstructions: diagnostics.EADS.TotalInstructions,
			TotalAPICalls:     diagnostics.EADS.TotalAPICalls,
			Events:            make([]eadsEventResult, 0, len(diagnostics.EADS.Events)),
		}
		for _, event := range diagnostics.EADS.Events {
			result.EADS.Events = append(result.EADS.Events, eadsEventResult{
				Event:        fmt.Sprintf("0x%04x", event.Event),
				Instructions: event.Instructions,
				APICalls:     event.APICalls,
				ReturnValue:  event.ReturnValue,
			})
		}
	}
	if diagnostics.WIPI != nil {
		result.WIPI = &wipiResult{
			PresentCount:        diagnostics.WIPI.PresentCount,
			APICalls:            diagnostics.WIPI.APICalls,
			ImplementedCalls:    diagnostics.WIPI.ImplementedCalls,
			UnimplementedCalls:  diagnostics.WIPI.UnimplementedCalls,
			LastAPI:             diagnostics.WIPI.LastAPI,
			LastUnimplemented:   diagnostics.WIPI.LastUnimplemented,
			CatalogedAPIs:       diagnostics.WIPI.CatalogedAPIs,
			DispatchWiredAPIs:   diagnostics.WIPI.DispatchWiredAPIs,
			SemanticallyModeled: diagnostics.WIPI.SemanticallyModeled,
			ObservedAPIs:        diagnostics.WIPI.ObservedAPIs,
			UnimplementedAPIs:   append([]string(nil), diagnostics.WIPI.UnimplementedAPIs...),
		}
	}
}

func classifyError(err error, format string) (string, string, frontend.FailureKind) {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "timeout", "", frontend.FailureUnknown
	}
	var backendErr *frontend.BackendError
	if errors.As(err, &backendErr) {
		switch backendErr.Kind {
		case frontend.FailureUnsupportedProfile:
			return "unsupported_format", "recognized", backendErr.Kind
		case frontend.FailureMalformedInput:
			return "malformed_input", recognizedLevel(format), backendErr.Kind
		case frontend.FailureGuestFaulted:
			if errors.Is(err, cpu.ErrUnsupportedInstruction) {
				return "unimplemented_instruction", "loads", backendErr.Kind
			}
			return "guest_fault", "loads", backendErr.Kind
		case frontend.FailureBackendUnavailable:
			return "backend_unavailable", "", backendErr.Kind
		default:
			return "runner_error", "", backendErr.Kind
		}
	}
	if errors.Is(err, cpu.ErrUnsupportedInstruction) {
		return "unimplemented_instruction", "loads", frontend.FailureGuestFaulted
	}
	return "runner_error", "", frontend.FailureUnknown
}

func recognizedLevel(format string) string {
	if format == "" || format == "unknown" {
		return ""
	}
	return "recognized"
}

func writeResult(result probeResult) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(result); err != nil {
		fmt.Fprintf(os.Stderr, "encode probe result: %v\n", err)
	}
}
