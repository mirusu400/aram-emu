package carrier

import (
	authd "github.com/mirusu400/aram-authd"
	"github.com/mirusu400/aram-core/netauth"
)

// AuthdRaptorNet adapts an aram-authd backend to the netauth.Backend seam used
// by aram-core's Raptor runtime. It is headless so every product host can share
// the same carrier behavior without importing the desktop frontend adapter.
func AuthdRaptorNet(backend authd.NetBackend) netauth.Backend {
	if backend == nil {
		return nil
	}
	return authdNetAdapter{backend: backend}
}

type authdNetAdapter struct{ backend authd.NetBackend }

func (a authdNetAdapter) Handle(call netauth.Call, memory netauth.Memory) (uint32, bool) {
	return a.backend.Handle(
		authd.Call{Ordinal: call.Ordinal, Args: call.Args},
		authdMemory{memory: memory},
	)
}

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

type authdMemory struct{ memory netauth.Memory }

func (m authdMemory) ReadU8(address uint32) (uint8, error) {
	return m.memory.ReadU8(address)
}

func (m authdMemory) WriteU8(address uint32, value uint8) error {
	return m.memory.WriteU8(address, value)
}

func (m authdMemory) ReadU32(address uint32) (uint32, error) {
	return m.memory.ReadU32(address)
}

func (m authdMemory) WriteU32(address uint32, value uint32) error {
	return m.memory.WriteU32(address, value)
}

func (m authdMemory) ReadBytes(address uint32, count int) ([]byte, error) {
	return m.memory.ReadBytes(address, count)
}

func (m authdMemory) WriteBytes(address uint32, data []byte) error {
	return m.memory.WriteBytes(address, data)
}
