package integration

import (
	"testing"
	"time"

	aramcore "github.com/mirusu400/aram-core/core"
	"github.com/mirusu400/aram-frontend/frontend"
)

type pointerMachine struct {
	aramcore.Machine
	events []aramcore.PointerInputEvent
}

func (m *pointerMachine) SupportsPointer() bool { return true }
func (m *pointerMachine) QueuePointer(e aramcore.PointerInputEvent) error {
	m.events = append(m.events, e)
	return nil
}
func TestBackendTranslatesOptionalPointerInput(t *testing.T) {
	machine := &pointerMachine{}
	backend := &Backend{machine: machine}
	if !backend.SupportsPointer() {
		t.Fatal("adapter hid pointer support")
	}
	event := frontend.PointerInputEvent{X: 31, Y: 47, Pressed: true, At: time.Second}
	if err := backend.QueuePointer(event); err != nil {
		t.Fatal(err)
	}
	if len(machine.events) != 1 || machine.events[0] != (aramcore.PointerInputEvent{X: 31, Y: 47, Pressed: true, At: time.Second}) {
		t.Fatalf("input=%v", machine.events)
	}
	backend.machine = nil
	if backend.SupportsPointer() || backend.QueuePointer(event) == nil {
		t.Fatal("empty backend accepted pointer")
	}
}
