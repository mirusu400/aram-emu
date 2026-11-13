package integration

import (
	"testing"

	aramcore "github.com/mirusu400/aram-core/core"
)

type textInputCoreMachine struct {
	aramcore.Machine
	active bool
}

func (machine *textInputCoreMachine) TextInputActive() bool { return machine.active }

func TestTextInputActiveForwardsCurrentMachineHint(t *testing.T) {
	machine := &textInputCoreMachine{active: true}
	backend := &Backend{machine: machine}
	if !backend.TextInputActive() {
		t.Fatal("active core text input was not forwarded")
	}
	machine.active = false
	if backend.TextInputActive() {
		t.Fatal("inactive core text input was reported active")
	}
}
