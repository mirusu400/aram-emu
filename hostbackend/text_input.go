package hostbackend

import "github.com/mirusu400/aram-frontend/frontend"

// TextInputActive forwards the hint from the adapter that owns the current
// input. Adapter selection is synchronized so an application-to-firmware switch
// cannot leak the outgoing title's input state.
func (b *Backend) TextInputActive() bool {
	target := b.active
	textInput, ok := target.(frontend.TextInputBackend)
	return ok && textInput.TextInputActive()
}

var _ frontend.TextInputBackend = (*Backend)(nil)
