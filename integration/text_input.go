package integration

import (
	aramcore "github.com/mirusu400/aram-core/core"
	"github.com/mirusu400/aram-frontend/frontend"
)

// TextInputActive translates the optional core presentation hint without
// exposing the concrete machine or its runtime to the shared frontend.
func (backend *Backend) TextInputActive() bool {
	backend.operationMu.Lock()
	defer backend.operationMu.Unlock()
	machine := backend.currentMachine()
	textInput, ok := machine.(aramcore.TextInputMachine)
	return ok && textInput.TextInputActive()
}

var _ frontend.TextInputBackend = (*Backend)(nil)
