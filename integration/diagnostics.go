package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"image/color"

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
	Java      *JavaDiagnostics
	GVM       *GVMDiagnostics
	BREW      *BREWDiagnostics
}

// GVMDiagnostics reports a service request observed by the bounded diagnostic
// profile. It does not claim that the service was delivered to the guest.
type GVMDiagnostics struct {
	Boundary              string
	Interval              int16
	Selector              uint16
	PresentCount          uint64
	FrameValid            bool
	FramebufferSHA256     string
	NonUniform            bool
	InputDispatchCount    uint64
	LastInputGuestCode    uint16
	LastInputInstructions uint64
}

// JavaDiagnostics describes the shared Java engine without manufacturing ARM
// register/entry information. A published frame is not proof of a booted title.
type JavaDiagnostics struct {
	Runtime           string
	MainClass         string
	Started           bool
	HasDisplay        bool
	Instructions      uint64
	PresentCount      uint64
	FramebufferSHA256 string
	FrameValid        bool
}

// BREWDiagnostics reports guest-owned presentation activity. A host framebuffer
// by itself is deliberately not evidence that the BREW guest drew.
type BREWDiagnostics struct {
	PresentCount uint64
	FrameValid   bool
}

type ImageDiagnostics struct {
	Name        string
	ProfileID   string
	SourceKind  string
	CPUBackend  string
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
	ObservedAPINames    []string
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
	// Reporting interfaces live on the core machine, not on the cheat wrapper
	// the backend publishes, and every probe below is read-only.
	machine = unwrapMachine(machine)
	if provider, ok := machine.(coreDebugSnapshotter); ok {
		debug := provider.DebugSnapshot(1)
		if debug.SKVM != nil {
			java := debug.SKVM
			snapshot.Java = &JavaDiagnostics{
				Runtime: debug.Runtime, MainClass: java.MainClass,
				Started: java.Started, Instructions: java.Instructions,
				HasDisplay: java.CurrentDisplay != 0,
			}
			if frame := java.Framebuffer; frame != nil {
				snapshot.Java.PresentCount = frame.Sequence
				snapshot.Java.FramebufferSHA256 = frame.RGBASHA256
				snapshot.Java.FrameValid = frame.SnapshotHashOK && frame.DescriptorValid
			}
		}
	}
	if provider, ok := machine.(interface {
		ImageInfo() application.ImageInfo
	}); ok {
		info := provider.ImageInfo()
		snapshot.Image = &ImageDiagnostics{
			Name:        info.Name,
			ProfileID:   info.ProfileID,
			SourceKind:  string(info.SourceKind),
			CPUBackend:  info.CPUBackend,
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
		GVMDiagnosticBoundary() (application.GVMDiagnosticBoundary, bool)
	}); ok {
		if boundary, present := provider.GVMDiagnosticBoundary(); present {
			snapshot.GVM = &GVMDiagnostics{
				Boundary: boundary.Kind,
				Interval: boundary.Interval,
				Selector: boundary.Selector,
			}
			if snapshot.Execution != nil && snapshot.State == frontend.StateStopped {
				snapshot.Execution.Reason = "service-boundary"
			}
		}
	}
	if provider, ok := machine.(application.GVMPresentDiagnostics); ok {
		if snapshot.GVM == nil {
			snapshot.GVM = &GVMDiagnostics{}
		}
		snapshot.GVM.PresentCount = provider.GVMPresentCount()
	}
	if provider, ok := machine.(interface{ GNEX32PresentCount() uint64 }); ok {
		if snapshot.GVM == nil {
			snapshot.GVM = &GVMDiagnostics{}
		}
		snapshot.GVM.PresentCount = provider.GNEX32PresentCount()
	}
	if snapshot.GVM != nil {
		if frame := machine.Framebuffer(); frame != nil {
			bounds := frame.Bounds()
			snapshot.GVM.FrameValid = bounds.Dx() > 0 && bounds.Dy() > 0
			if snapshot.GVM.FrameValid {
				hash := sha256.New()
				var first color.RGBA
				firstSet := false
				for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
					for x := bounds.Min.X; x < bounds.Max.X; x++ {
						pixel := color.RGBAModel.Convert(frame.At(x, y)).(color.RGBA)
						_, _ = hash.Write([]byte{pixel.R, pixel.G, pixel.B, pixel.A})
						if !firstSet {
							first, firstSet = pixel, true
						} else if pixel != first {
							snapshot.GVM.NonUniform = true
						}
					}
				}
				snapshot.GVM.FramebufferSHA256 = hex.EncodeToString(hash.Sum(nil))
			}
		}
	}
	if provider, ok := machine.(interface {
		GVMInputDispatchDiagnostics() application.GVMInputDispatchDiagnostics
	}); ok {
		if snapshot.GVM == nil {
			snapshot.GVM = &GVMDiagnostics{}
		}
		input := provider.GVMInputDispatchDiagnostics()
		snapshot.GVM.InputDispatchCount = input.DispatchCount
		snapshot.GVM.LastInputGuestCode = input.GuestCode
		snapshot.GVM.LastInputInstructions = input.Result.Instructions
	}
	if provider, ok := machine.(interface {
		WIPIFrameStats() (application.WIPIFrameStats, bool)
		WIPIAPICoverage() (application.WIPIAPICoverage, bool)
		WIPIObservedAPIs() []string
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
				ObservedAPINames:    provider.WIPIObservedAPIs(),
				UnimplementedAPIs:   provider.WIPIUnimplementedAPIs(),
			}
		}
	}
	if provider, ok := machine.(interface {
		BREWFrameStats() (application.BREWFrameStats, bool)
	}); ok {
		// This guest-owned counter is the sole source of BREW frame evidence.
		// Never infer a guest presentation from the host buffer.
		if stats, present := provider.BREWFrameStats(); present {
			snapshot.BREW = &BREWDiagnostics{
				PresentCount: stats.PresentCount,
				FrameValid:   stats.FrameValid,
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
