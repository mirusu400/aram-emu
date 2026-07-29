package integration

import (
	"github.com/mirusu400/aram-core/application"
	"github.com/mirusu400/aram-core/cpu"
	"github.com/mirusu400/aram-frontend/frontend"
)

// Diagnostics is a read-only, serialization-friendly view of the integrated
// machine. Compatibility tooling uses it instead of reaching into core
// internals or parsing human-readable debugger text.
type Diagnostics struct {
	State     frontend.BackendState
	Input     frontend.InputInfo
	Image     *ImageDiagnostics
	Execution *ExecutionDiagnostics
	WIPI      *WIPIDiagnostics
	EADS      *EADSDiagnostics
}

type ImageDiagnostics struct {
	Name        string
	ProfileID   string
	SourceKind  string
	EntryPoint  uint32
	Mode        string
	TextAddress uint32
	TextSize    uint32
	BSSAddress  uint32
	BSSSize     uint32
}

type ExecutionDiagnostics struct {
	Reason       string
	Instructions uint64
	PC           uint32
	Error        string
}

type WIPIDiagnostics struct {
	PresentCount        uint32
	APICalls            uint64
	ImplementedCalls    uint64
	UnimplementedCalls  uint64
	LastAPI             string
	LastUnimplemented   string
	CatalogedAPIs       int
	DispatchWiredAPIs   int
	SemanticallyModeled int
	ObservedAPIs        int
	UnimplementedAPIs   []string
}

type EADSDiagnostics struct {
	Events            []EADSEventDiagnostics
	PresentCount      uint32
	TickMS            uint32
	TotalInstructions uint64
	TotalAPICalls     uint64
}

type EADSEventDiagnostics struct {
	Event        uint32
	Instructions uint64
	APICalls     uint64
	ReturnValue  uint32
}

func (backend *Backend) Diagnostics() Diagnostics {
	backend.mu.RLock()
	input := backend.input
	machine := backend.machine
	backend.mu.RUnlock()

	snapshot := Diagnostics{
		State: backend.State(),
		Input: input,
	}
	if machine == nil {
		return snapshot
	}
	if provider, ok := machine.(interface {
		ImageInfo() application.ImageInfo
	}); ok {
		info := provider.ImageInfo()
		snapshot.Image = &ImageDiagnostics{
			Name:        info.Name,
			ProfileID:   info.ProfileID,
			SourceKind:  string(info.SourceKind),
			EntryPoint:  info.EntryPoint,
			Mode:        modeName(info.Mode),
			TextAddress: info.TextAddress,
			TextSize:    info.TextSize,
			BSSAddress:  info.BSSAddress,
			BSSSize:     info.BSSSize,
		}
	}
	if snapshot.State == frontend.StateReady {
		return snapshot
	}
	if provider, ok := machine.(interface {
		LastResult() cpu.Result
	}); ok {
		result := provider.LastResult()
		execution := &ExecutionDiagnostics{
			Reason:       stopReasonName(result.Reason),
			Instructions: result.Instructions,
			PC:           result.PC,
		}
		if result.Err != nil {
			execution.Error = result.Err.Error()
		}
		snapshot.Execution = execution
	}
	if provider, ok := machine.(interface {
		WIPIFrameStats() (application.WIPIFrameStats, bool)
		WIPIAPICoverage() (application.WIPIAPICoverage, bool)
		WIPIUnimplementedAPIs() []string
	}); ok {
		stats, present := provider.WIPIFrameStats()
		coverage, covered := provider.WIPIAPICoverage()
		if present && covered {
			snapshot.WIPI = &WIPIDiagnostics{
				PresentCount:        stats.PresentCount,
				APICalls:            stats.APICalls,
				ImplementedCalls:    stats.ImplementedCalls,
				UnimplementedCalls:  stats.UnimplementedCalls,
				LastAPI:             stats.LastAPI,
				LastUnimplemented:   stats.LastUnimplemented,
				CatalogedAPIs:       coverage.Cataloged,
				DispatchWiredAPIs:   coverage.DispatchWired,
				SemanticallyModeled: coverage.SemanticallyModeled,
				ObservedAPIs:        coverage.Observed,
				UnimplementedAPIs:   provider.WIPIUnimplementedAPIs(),
			}
		}
	}
	if provider, ok := machine.(interface {
		EADSFrameStats() (application.EADSFrameStats, bool)
	}); ok {
		stats, present := provider.EADSFrameStats()
		if present {
			eads := &EADSDiagnostics{
				PresentCount: stats.PresentCount,
				TickMS:       stats.TickMS,
				Events:       make([]EADSEventDiagnostics, 0, len(stats.Events)),
			}
			for _, event := range stats.Events {
				eads.Events = append(eads.Events, EADSEventDiagnostics{
					Event:        event.Event,
					Instructions: event.Instructions,
					APICalls:     event.APICalls,
					ReturnValue:  event.ReturnValue,
				})
				eads.TotalInstructions += event.Instructions
				eads.TotalAPICalls += event.APICalls
			}
			snapshot.EADS = eads
		}
	}
	return snapshot
}

func stopReasonName(reason cpu.StopReason) string {
	switch reason {
	case cpu.StopRequested:
		return "requested"
	case cpu.StopBreakpoint:
		return "breakpoint"
	case cpu.StopFault:
		return "fault"
	case cpu.StopBudget:
		return "budget"
	case cpu.StopExited:
		return "exited"
	default:
		return "unknown"
	}
}
