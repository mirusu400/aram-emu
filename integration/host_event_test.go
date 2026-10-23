package integration

import (
	"errors"
	"testing"

	aramcore "github.com/mirusu400/aram-core/core"
	"github.com/mirusu400/aram-frontend/frontend"
)

type hostEventMachine struct {
	aramcore.Machine
	events []aramcore.HostEvent
}

func (m *hostEventMachine) ReceiveHostEvent(event aramcore.HostEvent) error {
	m.events = append(m.events, event)
	return nil
}

func TestBackendTranslatesHostEvent(t *testing.T) {
	machine := &hostEventMachine{}
	backend := &Backend{machine: machine}
	packet := []byte{1, 2, 3}
	if err := backend.ReceiveHostEvent(frontend.HostEvent{
		Kind: frontend.HostEventNetworkReceive,
		Data: packet,
	}); err != nil {
		t.Fatal(err)
	}
	packet[0] = 9
	if len(machine.events) != 1 ||
		machine.events[0].Kind != aramcore.HostEventNetworkReceive ||
		machine.events[0].Data[0] != 1 {
		t.Fatalf("core events = %+v", machine.events)
	}
}

func TestBackendRejectsHostEventWithoutReceiver(t *testing.T) {
	backend := &Backend{machine: &pointerMachine{}}
	err := backend.ReceiveHostEvent(frontend.HostEvent{Kind: frontend.HostEventSMSReceive})
	if !errors.Is(err, frontend.ErrHostEventUnsupported) {
		t.Fatalf("unsupported event error = %v", err)
	}
}
