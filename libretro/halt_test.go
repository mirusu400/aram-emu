package libretro

import (
	"context"
	"errors"
	"image"
	"io"
	"testing"

	aramcore "github.com/mirusu400/aram-core/core"
)

type haltingFactory struct{ machine *haltingMachine }

func (f *haltingFactory) Create(context.Context, aramcore.Source) (aramcore.Machine, error) {
	return f.machine, nil
}

// haltingMachine exits on its first frame and, like the application-mode
// machine, rejects Start from the stopped state.
type haltingMachine struct {
	state  aramcore.State
	starts int
	steps  int
	resets int
	frame  *image.RGBA
}

func newHaltingMachine() *haltingMachine {
	return &haltingMachine{state: aramcore.StateReady, frame: image.NewRGBA(image.Rect(0, 0, 8, 8))}
}

func (m *haltingMachine) Load(context.Context, aramcore.Source) error { return nil }
func (m *haltingMachine) State() aramcore.State                       { return m.state }
func (m *haltingMachine) Start(context.Context) error {
	m.starts++
	if m.state != aramcore.StateReady && m.state != aramcore.StatePaused {
		return errors.New("start from " + m.state.String())
	}
	m.state = aramcore.StateStopped
	return nil
}
func (m *haltingMachine) Pause() error  { return nil }
func (m *haltingMachine) Resume() error { return nil }
func (m *haltingMachine) Stop() error   { return nil }
func (m *haltingMachine) Reset(context.Context) error {
	m.resets++
	m.state = aramcore.StateReady
	return nil
}
func (m *haltingMachine) StepFrame(context.Context) error {
	m.steps++
	if m.state == aramcore.StateStopped {
		return errors.New("step from stopped")
	}
	return nil
}
func (m *haltingMachine) QueueInput(aramcore.InputEvent) error { return nil }
func (m *haltingMachine) Framebuffer() image.Image             { return m.frame }
func (m *haltingMachine) DrainAudio() aramcore.AudioChunk      { return aramcore.AudioChunk{} }
func (m *haltingMachine) SaveState(io.Writer) error            { return nil }
func (m *haltingMachine) LoadState(io.Reader) error            { return nil }
func (m *haltingMachine) Close() error                         { return nil }

func TestRunHoldsFinalFrameAfterGuestExitsUntilReset(t *testing.T) {
	machine := newHaltingMachine()
	core := New(Config{Factory: &haltingFactory{machine: machine}})
	t.Cleanup(func() { _ = core.Close() })
	if err := core.LoadGame(context.Background(), "synthetic.dat", syntheticEADS()); err != nil {
		t.Fatal(err)
	}
	for frame := 0; frame < 3; frame++ {
		result, err := core.Run(context.Background(), 0)
		if err != nil {
			t.Fatalf("frame %d after guest exit: %v", frame, err)
		}
		if result.Frame.Width != 8 || result.Frame.Height != 8 {
			t.Fatalf("frame %d = %dx%d", frame, result.Frame.Width, result.Frame.Height)
		}
	}
	if machine.starts != 1 || machine.steps != 0 {
		t.Fatalf("starts=%d steps=%d after guest exit, want a single start and no steps", machine.starts, machine.steps)
	}
	if err := core.Reset(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := core.Run(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	if machine.resets != 1 || machine.starts != 2 {
		t.Fatalf("resets=%d starts=%d after reset, want the guest restarted once", machine.resets, machine.starts)
	}
}
