package hostbackend

import (
	"context"
	"testing"

	"github.com/mirusu400/aram-frontend/frontend"
)

type pointerAdapter struct {
	recordingAdapter
	events []frontend.PointerInputEvent
}

func (a *pointerAdapter) SupportsPointer() bool { return true }
func (a *pointerAdapter) QueuePointer(event frontend.PointerInputEvent) error {
	a.events = append(a.events, event)
	return nil
}
func TestProductRoutesPointerToActiveAdapter(t *testing.T) {
	application := &pointerAdapter{}
	system := &recordingAdapter{}
	backend := newBackend(application, system)
	event := frontend.PointerInputEvent{X: 31, Y: 47, Pressed: true}
	if !backend.SupportsPointer() {
		t.Fatal("product hid application pointer support")
	}
	if err := backend.QueuePointer(event); err != nil {
		t.Fatal(err)
	}
	if len(application.events) != 1 || application.events[0] != event {
		t.Fatalf("application input=%v", application.events)
	}
	if _, err := backend.Open(context.Background(), frontend.OpenRequest{Firmware: true}); err != nil {
		t.Fatal(err)
	}
	if backend.SupportsPointer() || backend.QueuePointer(event) == nil {
		t.Fatal("keypad-only system accepted pointer input")
	}
	if len(application.events) != 1 {
		t.Fatal("system pointer leaked into outgoing application")
	}
}
