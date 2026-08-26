// Package hostbackend presents one frontend backend that runs both kinds of
// input this product supports: an application title on the application
// machine, and a whole handset on the system machine.
//
// The two machines are separate implementations with separate adapters, and
// deliberately so - an application title is not a phone. But that is an
// implementation boundary, not something a person opening a file should have
// to know about, and a shell that can only reach one of them turns its own
// firmware menu entry into a button that always fails. This routes an open
// request to the adapter that owns it and forwards the rest of the frontend
// contract to whichever one holds the current input.
package hostbackend

import (
	"context"
	"errors"

	aramcore "github.com/mirusu400/aram-core/core"
	"github.com/mirusu400/aram-emu/integration"
	"github.com/mirusu400/aram-emu/systemintegration"
	"github.com/mirusu400/aram-frontend/frontend"
)

var errToolActionUnsupported = errors.New("this machine does not implement tool actions")

// Options configures the whole-phone half. The application half keeps the
// product defaults it has always had.
type Options struct {
	Factory aramcore.Factory
	System  systemintegration.Options
}

// Backend routes between the application and whole-phone adapters. The
// application adapter is active until a firmware directory is opened, so a
// session that never touches firmware behaves exactly as it did when the
// application adapter was the only backend the product built.
type Backend struct {
	// Both halves are held behind the frontend contract rather than as their
	// concrete adapters: routing is the only thing this type decides, and a
	// test can then check where a request went without building a machine.
	application frontend.Backend
	system      frontend.Backend
	active      frontend.Backend

	// The shell configures audio, font, and CPU once at startup against
	// whichever backend it was given. Remembering them here lets a switch
	// re-apply the same choices to the adapter that becomes active, instead
	// of dropping them silently at the boundary.
	audio *frontend.AudioSettings
	font  *frontend.FontSettings
	cpu   *frontend.CPUSettings
}

func NewBackend(options Options) *Backend {
	return newBackend(
		integration.NewBackend(options.Factory),
		systemintegration.NewBackend(options.System),
	)
}

func newBackend(application, system frontend.Backend) *Backend {
	return &Backend{application: application, system: system, active: application}
}

// SupportsFirmware reports the capability the shell uses to decide whether to
// offer its firmware command.
func (b *Backend) SupportsFirmware() bool { return true }

func (b *Backend) adapterFor(firmware bool) frontend.Backend {
	if firmware {
		return b.system
	}
	return b.application
}

// selectAdapter makes the adapter that owns this request the active one. The
// outgoing adapter is closed rather than left holding an input: Close releases
// the current input without ending the backend, and for the whole-phone
// adapter it is also what commits the writable NAND.
func (b *Backend) selectAdapter(firmware bool) (frontend.Backend, error) {
	next := b.adapterFor(firmware)
	if next == b.active {
		return next, nil
	}
	if err := b.active.Close(); err != nil {
		return nil, err
	}
	b.active = next
	b.applyRememberedSettings()
	return next, nil
}

func (b *Backend) applyRememberedSettings() {
	if b.audio != nil {
		if target, ok := b.active.(frontend.AudioBackend); ok {
			_ = target.ConfigureAudio(*b.audio)
		}
	}
	if b.font != nil {
		if target, ok := b.active.(frontend.FontBackend); ok {
			_ = target.ConfigureFont(*b.font)
		}
	}
	if b.cpu != nil {
		if target, ok := b.active.(frontend.CPUBackendSelector); ok {
			_ = target.ConfigureCPU(*b.cpu)
		}
	}
}

func (b *Backend) Open(
	ctx context.Context,
	request frontend.OpenRequest,
) (frontend.InputInfo, error) {
	target, err := b.selectAdapter(request.Firmware)
	if err != nil {
		return frontend.InputInfo{}, err
	}
	return target.Open(ctx, request)
}

func (b *Backend) OpenWithProgress(
	ctx context.Context,
	request frontend.OpenRequest,
	progress func(frontend.OpenStage),
) (frontend.InputInfo, error) {
	target, err := b.selectAdapter(request.Firmware)
	if err != nil {
		return frontend.InputInfo{}, err
	}
	if opener, ok := target.(frontend.OpenProgressBackend); ok {
		return opener.OpenWithProgress(ctx, request, progress)
	}
	return target.Open(ctx, request)
}

func (b *Backend) State() frontend.BackendState { return b.active.State() }

func (b *Backend) Supports(command frontend.BackendCommand) bool {
	return b.active.Supports(command)
}

func (b *Backend) Execute(ctx context.Context, command frontend.BackendCommand) error {
	return b.active.Execute(ctx, command)
}

// Close releases the current input. The frontend uses it both to close a title
// and to shut down, so it does not change which adapter is active: the next
// open picks that again from the request.
func (b *Backend) Close() error { return b.active.Close() }

func (b *Backend) Capability(command frontend.BackendCommand) frontend.Capability {
	if target, ok := b.active.(frontend.CapabilityBackend); ok {
		return target.Capability(command)
	}
	if b.active.Supports(command) {
		return frontend.Capability{Supported: true}
	}
	return frontend.Capability{}
}

func (b *Backend) ExecuteCommand(ctx context.Context, request frontend.CommandRequest) error {
	if target, ok := b.active.(frontend.CommandBackend); ok {
		return target.ExecuteCommand(ctx, request)
	}
	return b.active.Execute(ctx, request.Command)
}

func (b *Backend) VideoFrame() frontend.VideoFrame {
	if target, ok := b.active.(frontend.VideoBackend); ok {
		return target.VideoFrame()
	}
	return frontend.VideoFrame{}
}

func (b *Backend) RunFrame(ctx context.Context) error {
	if target, ok := b.active.(frontend.FrameBackend); ok {
		return target.RunFrame(ctx)
	}
	return nil
}

func (b *Backend) QueueInput(event frontend.InputEvent) error {
	if target, ok := b.active.(frontend.InputBackend); ok {
		return target.QueueInput(event)
	}
	return nil
}

// ConfigureAudio, ConfigureFont, and ConfigureCPU record the choice even when
// the active adapter cannot act on it, so switching to one that can does not
// need the shell to configure it a second time.
func (b *Backend) ConfigureAudio(settings frontend.AudioSettings) error {
	b.audio = &settings
	if target, ok := b.active.(frontend.AudioBackend); ok {
		return target.ConfigureAudio(settings)
	}
	return nil
}

func (b *Backend) ConfigureFont(settings frontend.FontSettings) error {
	b.font = &settings
	if target, ok := b.active.(frontend.FontBackend); ok {
		return target.ConfigureFont(settings)
	}
	return nil
}

func (b *Backend) ConfigureCPU(settings frontend.CPUSettings) error {
	b.cpu = &settings
	if target, ok := b.active.(frontend.CPUBackendSelector); ok {
		return target.ConfigureCPU(settings)
	}
	return nil
}

func (b *Backend) AvailableCPUBackends() []string {
	if target, ok := b.active.(frontend.CPUBackendSelector); ok {
		return target.AvailableCPUBackends()
	}
	return nil
}

func (b *Backend) DrainAudio() frontend.AudioChunk {
	if target, ok := b.active.(frontend.AudioStreamBackend); ok {
		return target.DrainAudio()
	}
	return frontend.AudioChunk{}
}

func (b *Backend) ToolSnapshot(
	ctx context.Context,
	kind frontend.ToolKind,
) (frontend.ToolSnapshot, error) {
	if target, ok := b.active.(frontend.ToolBackend); ok {
		return target.ToolSnapshot(ctx, kind)
	}
	return frontend.ToolSnapshot{}, nil
}

func (b *Backend) ExecuteToolAction(
	ctx context.Context,
	request frontend.ToolRequest,
) (frontend.ToolSnapshot, error) {
	if target, ok := b.active.(frontend.ToolActionBackend); ok {
		return target.ExecuteToolAction(ctx, request)
	}
	return frontend.ToolSnapshot{}, errToolActionUnsupported
}

func (b *Backend) DebugArtifacts(ctx context.Context) ([]frontend.DebugArtifact, error) {
	if target, ok := b.active.(frontend.DebugExportBackend); ok {
		return target.DebugArtifacts(ctx)
	}
	return nil, nil
}

func (b *Backend) BackendName() string {
	if target, ok := b.active.(frontend.BackendNamer); ok {
		return target.BackendName()
	}
	return "aram"
}

// DefaultSystemOptions selects the fastest backend registered by this build.
// systemintegration resolves that portable name to native, Go-JIT, or precise
// when the firmware machine is created.
func DefaultSystemOptions() systemintegration.Options {
	return systemintegration.Options{}
}

var _ frontend.Backend = (*Backend)(nil)
var _ frontend.FirmwareBackend = (*Backend)(nil)
