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
	timeout := flag.Duration("timeout", 10*time.Second, "whole-probe timeout")
	flag.Parse()

	started := time.Now()
	result := probeResult{
		Schema: resultSchema,
		Status: "runner_error",
		State:  frontend.StateEmpty,
	}
	if *input == "" || *slices == 0 || *timeout <= 0 {
		result.Detail = "input, positive slices, and positive timeout are required"
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
	for range *slices {
		err = backend.Execute(ctx, frontend.CommandStart)
		diagnostics := backend.Diagnostics()
		result.State = diagnostics.State
		copyDiagnostics(&result, diagnostics)
		if diagnostics.EADS != nil {
			result.TotalInstructions = diagnostics.EADS.TotalInstructions
		} else if diagnostics.Execution != nil {
			result.TotalInstructions += diagnostics.Execution.Instructions
		}
		if err != nil {
			result.Status, _, result.ErrorKind = classifyError(err, result.Format)
			result.Detail = err.Error()
			break
		}
		if (diagnostics.EADS != nil && diagnostics.EADS.PresentCount > 0) ||
			(diagnostics.WIPI != nil && diagnostics.WIPI.PresentCount > 0) {
			result.Status = "ok_frame"
			result.Level = "boots"
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
