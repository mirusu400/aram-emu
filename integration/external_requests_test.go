package integration

import (
	"testing"

	aramcore "github.com/mirusu400/aram-core/core"
	"github.com/mirusu400/aram-frontend/frontend"
)

type externalRequestMachine struct {
	aramcore.Machine
	requests []aramcore.ExternalRequest
}

func (machine *externalRequestMachine) ExternalRequests() []aramcore.ExternalRequest {
	result := append([]aramcore.ExternalRequest(nil), machine.requests...)
	for index := range result {
		result[index].Data = append([]byte(nil), result[index].Data...)
	}
	return result
}

func (machine *externalRequestMachine) ClearExternalRequests(through uint64) {
	for len(machine.requests) != 0 && machine.requests[0].Sequence <= through {
		machine.requests = machine.requests[1:]
	}
}

func TestBackendBridgesExternalRequestOutbox(t *testing.T) {
	machine := &externalRequestMachine{requests: []aramcore.ExternalRequest{{
		Sequence: 7,
		GuestNS:  123,
		Kind:     aramcore.ExternalRequestBrowser,
		Target:   "https://example.invalid/play",
		Data:     []byte{1, 2, 3},
	}}}
	backend := &Backend{machine: machine}
	requests := backend.ExternalRequests()
	if len(requests) != 1 ||
		requests[0].Sequence != 7 ||
		requests[0].GuestNS != 123 ||
		requests[0].Kind != frontend.ExternalRequestBrowser ||
		requests[0].Target != "https://example.invalid/play" {
		t.Fatalf("frontend requests = %+v", requests)
	}
	requests[0].Data[0] = 9
	if machine.requests[0].Data[0] != 1 {
		t.Fatal("frontend request aliased core-owned data")
	}
	backend.ClearExternalRequests(7)
	if len(machine.requests) != 0 {
		t.Fatalf("request was not acknowledged: %+v", machine.requests)
	}
}

var _ aramcore.ExternalRequestMachine = (*externalRequestMachine)(nil)
