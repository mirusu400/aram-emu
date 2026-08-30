package integration

import (
	authd "github.com/mirusu400/aram-authd"
	"github.com/mirusu400/aram-core/netauth"
)

// AuthdRaptorNet adapts an aram-authd backend to the netauth.Backend seam the
// aram-core raptor runtime expects, so the composition root can inject the LGT
// carrier DRM/auth emulator via application.Factory.RaptorNet. Returns nil for
// a nil backend so callers can pass an optional backend straight through.
func AuthdRaptorNet(backend authd.NetBackend) netauth.Backend {
	if backend == nil {
		return nil
	}
	return authdNetAdapter{backend: backend}
}

type authdNetAdapter struct{ backend authd.NetBackend }

func (a authdNetAdapter) Handle(call netauth.Call, mem netauth.Memory) (uint32, bool) {
	return a.backend.Handle(
		authd.Call{Ordinal: call.Ordinal, Args: call.Args},
		authdMemory{mem: mem},
	)
}

// Complete forwards an aram-authd backend's asynchronous carrier completion to
// the netauth seam. Backends that do not emulate the handshake response (Nop,
// Recorder) do not implement authd.CompletionSource, so this returns nil and
// the runtime keeps its default behavior.
func (a authdNetAdapter) Complete(call netauth.Call) *netauth.Completion {
	source, ok := a.backend.(authd.CompletionSource)
	if !ok {
		return nil
	}
	completion := source.Complete(authd.Call{Ordinal: call.Ordinal, Args: call.Args})
	if completion == nil {
		return nil
	}
	return &netauth.Completion{
		Event:       completion.Event,
		Arg1:        completion.Arg1,
		Response:    completion.Response,
		DelayFrames: completion.DelayFrames,
	}
}

// authdMemory bridges netauth.Memory to authd.Memory (identical method sets).
type authdMemory struct{ mem netauth.Memory }

func (m authdMemory) ReadU8(addr uint32) (uint8, error)   { return m.mem.ReadU8(addr) }
func (m authdMemory) WriteU8(addr uint32, v uint8) error  { return m.mem.WriteU8(addr, v) }
func (m authdMemory) ReadU32(addr uint32) (uint32, error) { return m.mem.ReadU32(addr) }
func (m authdMemory) WriteU32(addr uint32, v uint32) error {
	return m.mem.WriteU32(addr, v)
}
func (m authdMemory) ReadBytes(addr uint32, n int) ([]byte, error) {
	return m.mem.ReadBytes(addr, n)
}
func (m authdMemory) WriteBytes(addr uint32, d []byte) error {
	return m.mem.WriteBytes(addr, d)
}
