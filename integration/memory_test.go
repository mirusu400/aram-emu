package integration

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/mirusu400/aram-core/application"
	"github.com/mirusu400/aram-core/cheat"
	"github.com/mirusu400/aram-frontend/frontend"
)

// Synthetic Thumb guest increments a counter once per four-instruction frame.
func syntheticMemoryCounter() []byte {
	data := append(syntheticEADS()[:0xb0], make([]byte, 16)...)
	binary.LittleEndian.PutUint32(data[0x90:], 16)
	copy(data[0xb0:], []byte{0, 0xb5, 2, 0x48, 1, 0x68, 1, 0x31, 1, 0x60, 0xfb, 0xe7, 0, 0, 0, 3})
	return data
}

func memoryCounterBackend(t *testing.T) *Backend {
	t.Helper()
	factory := application.NewFactory()
	factory.RunBudget, factory.FrameRunBudget = 2, 4
	backend := NewBackend(factory)
	backend.cheatStore.cacheRoot = t.TempDir()
	path := filepath.Join(t.TempDir(), "counter.dat")
	if err := os.WriteFile(path, syntheticMemoryCounter(), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Open(context.Background(), frontend.OpenRequest{Path: path}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	return backend
}

func memoryAction(t *testing.T, backend *Backend, snapshot frontend.ToolSnapshot, action string, fields map[string]string) frontend.ToolSnapshot {
	t.Helper()
	next, err := backend.ExecuteToolAction(context.Background(), frontend.ToolRequest{Kind: frontend.ToolMemory, Session: snapshot.Session, Action: action, Fields: fields})
	if err != nil {
		t.Fatalf("%s: %v", action, err)
	}
	return next
}

func TestMemoryProductScanGuestFrameRefineWriteAndConflict(t *testing.T) {
	backend := memoryCounterBackend(t)
	snapshot, err := backend.ToolSnapshot(context.Background(), frontend.ToolMemory)
	if err != nil {
		t.Fatal(err)
	}
	snapshot = memoryAction(t, backend, snapshot, "scan", map[string]string{"type": "u32", "comparison": "unknown", "region": "image.data"})
	if snapshot.Memory.Total != 1024 || len(snapshot.Memory.Results) != memoryPageSize {
		t.Fatalf("first scan = %+v", snapshot.Memory)
	}
	if err := backend.Execute(context.Background(), frontend.CommandStart); err != nil {
		t.Fatal(err)
	}
	if err := backend.RunFrame(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Closing/reopening the panel performs a snapshot read and retains baseline.
	snapshot, err = backend.ToolSnapshot(context.Background(), frontend.ToolMemory)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Memory.Results[0].Value != "1" {
		t.Fatalf("guest did not increment counter: %+v", snapshot.Memory.Results[0])
	}
	snapshot = memoryAction(t, backend, snapshot, "refine", map[string]string{"comparison": "increased"})
	if snapshot.Memory.Total != 1 || snapshot.Memory.Results[0].Address != 0x03000000 {
		t.Fatalf("next scan = %+v", snapshot.Memory)
	}
	snapshot = memoryAction(t, backend, snapshot, "select", map[string]string{"address": "0x03000000"})
	selected := *snapshot.Memory.Selected
	snapshot = memoryAction(t, backend, snapshot, "write", map[string]string{"address": "0x03000000", "expected": selected.Expected, "new_value": "0x64"})
	if snapshot.Memory.Selected.Value != "100" {
		t.Fatalf("write = %+v", snapshot.Memory.Selected)
	}
	staleExpected := snapshot.Memory.Selected.Expected
	if err := backend.RunFrame(context.Background()); err != nil {
		t.Fatal(err)
	}
	next, err := backend.ExecuteToolAction(context.Background(), frontend.ToolRequest{
		Kind: frontend.ToolMemory, Session: snapshot.Session, Action: "write",
		Fields: map[string]string{"address": "0x03000000", "expected": staleExpected, "new_value": "200"},
	})
	if !errors.Is(err, cheat.ErrUnexpectedOriginal) || next.Memory.Selected.Value != "101" {
		t.Fatalf("conflict = %v %+v", err, next.Memory)
	}
}

func TestMemoryTypedParsingRangesAndFormats(t *testing.T) {
	for _, test := range []struct {
		kind, text string
		want       cheat.Value
	}{
		{"u8", "0xff", cheat.U8(255)}, {"i8", "-0x80", cheat.I8(-128)},
		{"u16", "65535", cheat.U16(65535)}, {"i16", "-32768", cheat.I16(-32768)},
		{"u32", "+0xFFFFFFFF", cheat.U32(0xffffffff)}, {"i32", "-0x80000000", cheat.I32(-2147483648)},
		{"u64", "18446744073709551615", cheat.U64(^uint64(0))}, {"i64", "-9223372036854775808", cheat.I64(-9223372036854775808)},
		{"f32", "-1.25", cheat.F32(-1.25)}, {"f64", "2.5e4", cheat.F64(25000)},
		{"u8", "010", cheat.U8(10)}, {"i32", "+42", cheat.I32(42)},
	} {
		value, err := parseMemoryValue(test.kind, test.text)
		if err != nil || value != test.want {
			t.Fatalf("%s %s: %+v %v", test.kind, test.text, value, err)
		}
		roundTrip, err := parseMemoryValue(test.kind, formatMemoryValue(value))
		if err != nil || roundTrip != value {
			t.Fatalf("format roundtrip %v: %v", value, err)
		}
	}
	for _, test := range [][2]string{{"u8", "256"}, {"u8", "-1"}, {"i8", "128"}, {"i16", "-32769"}, {"u64", "18446744073709551616"}, {"i64", "9223372036854775808"}, {"f32", "1e100"}, {"f64", "NaN"}, {"f64", "Inf"}, {"f64", ""}, {"u32", "0x"}, {"u32", "oops"}, {"bad", "1"}} {
		if _, err := parseMemoryValue(test[0], test[1]); err == nil {
			t.Fatalf("invalid value accepted: %v", test)
		}
	}
	for _, address := range []string{"-1", "0x100000000", "garbage"} {
		if _, err := parseMemoryAddress(address); err == nil {
			t.Fatalf("invalid address %s", address)
		}
	}
}

func TestMemoryProductNumericComparisonsAndEmptyResults(t *testing.T) {
	for _, kind := range []string{"u8", "i8", "u16", "i16", "u32", "i32", "u64", "i64", "f32", "f64"} {
		t.Run(kind, func(t *testing.T) {
			backend := memoryCounterBackend(t)
			library, _ := backend.cheatLibrary()
			engine := library.Engine()
			valueType, _ := memoryType(kind)
			negative := kind[0] != 'u'
			initial, increased, decreased := "5", "9", "1"
			if negative {
				initial, increased, decreased = "-5", "-1", "-9"
			}
			value, _ := parseMemoryValue(kind, initial)
			for i := 0; i < 3; i++ {
				if err := engine.Write(0x03000000+uint32(i*valueType.Size()), value, nil); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, _ := backend.ToolSnapshot(context.Background(), frontend.ToolMemory)
			for _, comparison := range []string{"increased", "decreased", "changed", "unchanged"} {
				for i := 0; i < 3; i++ {
					if err := engine.Write(0x03000000+uint32(i*valueType.Size()), value, nil); err != nil {
						t.Fatal(err)
					}
				}
				snapshot = memoryAction(t, backend, snapshot, "scan", map[string]string{"type": kind, "region": "image.data", "comparison": "equal", "value": initial})
				for i, text := range []string{increased, decreased} {
					changed, _ := parseMemoryValue(kind, text)
					if err := engine.Write(0x03000000+uint32(i*valueType.Size()), changed, nil); err != nil {
						t.Fatal(err)
					}
				}
				snapshot = memoryAction(t, backend, snapshot, "refine", map[string]string{"comparison": comparison})
				want := 1
				if comparison == "changed" {
					want = 2
				}
				if snapshot.Memory.Total != want {
					t.Fatalf("%s: got %d want %d", comparison, snapshot.Memory.Total, want)
				}
			}
			snapshot = memoryAction(t, backend, snapshot, "refine", map[string]string{"comparison": "equal", "value": "123"})
			if snapshot.Memory.Total != 0 || len(snapshot.Memory.Results) != 0 {
				t.Fatalf("empty scan = %+v", snapshot.Memory)
			}
			snapshot = memoryAction(t, backend, snapshot, "reset", nil)
			if snapshot.Memory.Active {
				t.Fatal("reset kept scan")
			}
		})
	}
}

func TestMemoryLifecycleRejectsStaleRequestsAndResetsBaseline(t *testing.T) {
	backend := memoryCounterBackend(t)
	if err := backend.ConfigureStateRoot(t.TempDir()); err == nil {
		t.Fatal("state root changed while loaded")
	}
	// Default state root is redirected process-locally through the backend.
	backend.mu.Lock()
	backend.stateRoot = t.TempDir()
	backend.mu.Unlock()
	for _, action := range []func() error{
		func() error { return backend.Execute(context.Background(), frontend.CommandReset) },
		func() error {
			if err := backend.Execute(context.Background(), frontend.CommandSaveState); err != nil {
				return err
			}
			return backend.Execute(context.Background(), frontend.CommandLoadState)
		},
		func() error {
			request := frontend.OpenRequest{Data: syntheticMemoryCounter(), DisplayName: "replacement.dat"}
			_, err := backend.Open(context.Background(), request)
			return err
		},
	} {
		snapshot, err := backend.ToolSnapshot(context.Background(), frontend.ToolMemory)
		if err != nil {
			t.Fatal(err)
		}
		snapshot = memoryAction(t, backend, snapshot, "scan", map[string]string{"type": "u32", "comparison": "unknown", "region": "image.data"})
		if err := action(); err != nil {
			t.Fatal(err)
		}
		next, err := backend.ExecuteToolAction(context.Background(), frontend.ToolRequest{Kind: frontend.ToolMemory, Session: snapshot.Session, Action: "refine", Fields: map[string]string{"comparison": "changed"}})
		if !errors.Is(err, errMemorySessionChanged) || next.Memory.Active {
			t.Fatalf("stale request: %+v %v", next.Memory, err)
		}
	}
	if err := backend.Close(); err != nil {
		t.Fatal(err)
	}
	snapshot, err := backend.ToolSnapshot(context.Background(), frontend.ToolMemory)
	if err != nil || snapshot.Memory != nil {
		t.Fatalf("closed tool: %+v %v", snapshot, err)
	}
}

func TestMemoryRequestsSerializeWithFrames(t *testing.T) {
	backend := memoryCounterBackend(t)
	snapshot, _ := backend.ToolSnapshot(context.Background(), frontend.ToolMemory)
	snapshot = memoryAction(t, backend, snapshot, "scan", map[string]string{"type": "u32", "comparison": "unknown", "region": "image.data"})
	snapshot = memoryAction(t, backend, snapshot, "select", map[string]string{"address": "0x03000000"})
	if err := backend.Execute(context.Background(), frontend.CommandStart); err != nil {
		t.Fatal(err)
	}
	// Contended frame execution and live-page reads use the ordinary backend.
	var group sync.WaitGroup
	errorsOut := make(chan error, 120)
	for i := 0; i < 40; i++ {
		group.Add(3)
		go func() { defer group.Done(); errorsOut <- backend.RunFrame(context.Background()) }()
		go func() {
			defer group.Done()
			_, err := backend.ToolSnapshot(context.Background(), frontend.ToolMemory)
			errorsOut <- err
		}()
		go func() {
			defer group.Done()
			current, err := backend.ToolSnapshot(context.Background(), frontend.ToolMemory)
			if err != nil {
				errorsOut <- err
				return
			}
			selected := current.Memory.Selected
			_, err = backend.ExecuteToolAction(context.Background(), frontend.ToolRequest{
				Kind: frontend.ToolMemory, Session: current.Session, Action: "write",
				Fields: map[string]string{"address": "0x03000000", "expected": selected.Expected, "new_value": selected.Value},
			})
			// Reapplying the displayed value may conflict with a frame, but must
			// never replace a newer counter with an earlier displayed value.
			if errors.Is(err, cheat.ErrUnexpectedOriginal) {
				err = nil
			}
			errorsOut <- err
		}()
	}
	group.Wait()
	close(errorsOut)
	for err := range errorsOut {
		if err != nil {
			t.Fatal(err)
		}
	}
	next, err := backend.ToolSnapshot(context.Background(), frontend.ToolMemory)
	if err != nil || next.Memory.Results[0].Value != "40" {
		t.Fatalf("serialized counter = %+v, %v", next.Memory, err)
	}
	t.Log(fmt.Sprintf("40 contended frames, snapshots, and checked writes completed"))
}
