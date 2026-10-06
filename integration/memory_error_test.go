package integration

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/mirusu400/aram-core/cheat"
	"github.com/mirusu400/aram-frontend/frontend"
)

type scalarTestMemory struct {
	data     [16]byte
	failRead bool
}

func (m *scalarTestMemory) ReadMemory(address uint32, destination []byte) error {
	if m.failRead {
		return errors.New("synthetic read failure")
	}
	if address < 0x1000 || uint64(address)+uint64(len(destination)) > 0x1010 {
		return errors.New("outside synthetic memory")
	}
	copy(destination, m.data[int(address-0x1000):])
	return nil
}
func (m *scalarTestMemory) WriteMemory(address uint32, source []byte) error {
	if address < 0x1000 || uint64(address)+uint64(len(source)) > 0x1010 {
		return errors.New("outside synthetic memory")
	}
	copy(m.data[int(address-0x1000):], source)
	return nil
}

func TestMemoryProductReadOnlyErrorsAndInputValidation(t *testing.T) {
	memory := &scalarTestMemory{}
	engine, err := cheat.New(memory, cheat.Options{Regions: []cheat.Region{{Name: "rom", Start: 0x1000, Size: 16, Scannable: true}}})
	if err != nil {
		t.Fatal(err)
	}
	library, err := cheat.NewLibrary(engine)
	if err != nil {
		t.Fatal(err)
	}
	backend := NewBackend(nil)
	backend.cheats, backend.memorySession = library, 1
	snapshot, err := backend.ToolSnapshot(context.Background(), frontend.ToolMemory)
	if err != nil {
		t.Fatal(err)
	}
	snapshot = memoryAction(t, backend, snapshot, "scan", map[string]string{"type": "u8", "comparison": "equal", "value": "0", "region": "rom"})
	snapshot = memoryAction(t, backend, snapshot, "select", map[string]string{"address": "0x1000"})
	if snapshot.Memory.Selected.Writable {
		t.Fatal("read-only region shown as writable")
	}
	next, err := backend.ExecuteToolAction(context.Background(), frontend.ToolRequest{Kind: frontend.ToolMemory, Session: snapshot.Session, Action: "write", Fields: map[string]string{"address": "0x1000", "expected": "0", "new_value": "1"}})
	if !errors.Is(err, cheat.ErrReadOnlyRegion) || next.Memory.Selected.Value != "0" {
		t.Fatalf("read-only write: %+v %v", next.Memory, err)
	}
	for _, request := range []frontend.ToolRequest{
		{Action: "select", Fields: map[string]string{"address": "0xFFFFFFFF"}},
		{Action: "write", Fields: map[string]string{"address": "0x100000000", "expected": "0", "new_value": "1"}},
		{Action: "refine", Fields: map[string]string{"comparison": "unknown"}},
		{Action: "scan", Fields: map[string]string{"comparison": "equal", "value": ""}},
		{Action: "scan", Fields: map[string]string{"comparison": "increased"}},
		{Action: "scan", Fields: map[string]string{"comparison": "invalid"}},
		{Action: "scan", Fields: map[string]string{"comparison": "unknown", "region": "missing"}},
		{Action: "refine", Fields: map[string]string{"comparison": "changed", "region": "rom", "type": "f64"}},
		{Action: "unexpected"},
	} {
		request.Kind, request.Session = frontend.ToolMemory, snapshot.Session
		if _, err := backend.ExecuteToolAction(context.Background(), request); err == nil {
			t.Fatalf("invalid request accepted: %+v", request)
		}
	}
	memory.failRead = true
	if _, err := backend.ToolSnapshot(context.Background(), frontend.ToolMemory); err == nil {
		t.Fatal("read error was hidden")
	}
}

func BenchmarkMemory32MiBPage(b *testing.B) {
	backend := NewBackend(nil)
	backend.cheatStore.cacheRoot = b.TempDir()
	if _, err := backend.Open(context.Background(), frontend.OpenRequest{Data: syntheticMemoryCounter(), DisplayName: "counter.dat"}); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = backend.Close() })
	snapshot, err := backend.ToolSnapshot(context.Background(), frontend.ToolMemory)
	if err != nil {
		b.Fatal(err)
	}
	snapshot, err = backend.ExecuteToolAction(context.Background(), frontend.ToolRequest{Kind: frontend.ToolMemory, Session: snapshot.Session, Action: "scan", Fields: map[string]string{"type": "u32", "comparison": "unknown", "region": "wipi.heap"}})
	if err != nil {
		b.Fatal(err)
	}
	if snapshot.Memory.Total != 8_388_608 || len(snapshot.Memory.Results) != memoryPageSize {
		b.Fatal(fmt.Sprintf("bounded page: %+v", snapshot.Memory))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := backend.ToolSnapshot(context.Background(), frontend.ToolMemory); err != nil {
			b.Fatal(err)
		}
	}
}
