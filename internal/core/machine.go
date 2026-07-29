package core

import (
	"context"
	"errors"
	"image"
	"io"
	"time"
)

// ErrBackendUnavailable is returned when a frontend command is valid but no
// application or firmware execution backend has been linked yet.
var ErrBackendUnavailable = errors.New("emulation backend unavailable")

// State is the backend-neutral lifecycle exposed to every frontend.
type State uint8

const (
	StateEmpty State = iota
	StateReady
	StateRunning
	StatePaused
	StateStopped
	StateFaulted
)

func (s State) String() string {
	switch s {
	case StateEmpty:
		return "empty"
	case StateReady:
		return "ready"
	case StateRunning:
		return "running"
	case StatePaused:
		return "paused"
	case StateStopped:
		return "stopped"
	case StateFaulted:
		return "faulted"
	default:
		return "unknown"
	}
}

// Source identifies user-supplied input without exposing frontend state to a
// machine implementation.
type Source struct {
	Path      string
	Format    string
	SHA256    string
	ProfileID string
}

// InputEvent is timestamped in virtual machine time for deterministic replay.
type InputEvent struct {
	Control string
	Pressed bool
	At      time.Duration
}

// AudioChunk is signed, interleaved little-endian PCM.
type AudioChunk struct {
	SampleRate int
	Channels   int
	PCM16      []int16
}

// Machine is the shared contract for application-HLE and firmware-system
// backends. Frontends must not reach around this interface into CPU state.
type Machine interface {
	Load(context.Context, Source) error
	State() State
	Start(context.Context) error
	Pause() error
	Resume() error
	Stop() error
	Reset(context.Context) error
	StepFrame(context.Context) error
	QueueInput(InputEvent) error
	Framebuffer() image.Image
	DrainAudio() AudioChunk
	SaveState(io.Writer) error
	LoadState(io.Reader) error
	Close() error
}

// Factory selects a machine implementation after format and profile
// detection. It allows the desktop shell to compile without a CPU backend.
type Factory interface {
	Create(context.Context, Source) (Machine, error)
}

// UnavailableFactory keeps inspection and configuration usable before a real
// execution backend is present.
type UnavailableFactory struct{}

func (UnavailableFactory) Create(context.Context, Source) (Machine, error) {
	return nil, ErrBackendUnavailable
}
