package integration

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	authd "github.com/mirusu400/aram-authd"
	"github.com/mirusu400/aram-core/application"
	machinecore "github.com/mirusu400/aram-core/core"
	"github.com/mirusu400/aram-core/netauth"
)

// netMemStub is a tiny netauth.Memory for the forwarding test.
type netMemStub map[uint32]byte

func (m netMemStub) ReadU8(a uint32) (uint8, error)  { return m[a], nil }
func (m netMemStub) WriteU8(a uint32, v uint8) error { m[a] = v; return nil }
func (m netMemStub) ReadU32(a uint32) (uint32, error) {
	return uint32(m[a]) | uint32(m[a+1])<<8 | uint32(m[a+2])<<16 | uint32(m[a+3])<<24, nil
}
func (m netMemStub) WriteU32(a uint32, v uint32) error {
	m[a], m[a+1], m[a+2], m[a+3] = byte(v), byte(v>>8), byte(v>>16), byte(v>>24)
	return nil
}
func (m netMemStub) ReadBytes(a uint32, n int) ([]byte, error) {
	b := make([]byte, n)
	for i := range b {
		b[i] = m[a+uint32(i)]
	}
	return b, nil
}
func (m netMemStub) WriteBytes(a uint32, d []byte) error {
	for i, v := range d {
		m[a+uint32(i)] = v
	}
	return nil
}

// TestAuthdRaptorNetAdapterForwards verifies the emu adapter translates a
// netauth call/memory to an authd backend and back.
func TestAuthdRaptorNetAdapterForwards(t *testing.T) {
	rec := authd.NewRecorder(nil)
	backend := AuthdRaptorNet(rec)
	if backend == nil {
		t.Fatal("AuthdRaptorNet returned nil for a non-nil backend")
	}
	mem := netMemStub{}
	if _, handled := backend.Handle(netauth.Call{Ordinal: 106, Args: [3]uint32{0xa600, 0, 0}}, mem); handled {
		t.Fatal("Recorder-backed adapter should decline (Nop inner)")
	}
	events := rec.Events()
	if len(events) != 1 || events[0].Ordinal != 106 || events[0].Args[0] != 0xa600 {
		t.Fatalf("adapter did not forward the call: %+v", events)
	}
	if AuthdRaptorNet(nil) != nil {
		t.Fatal("AuthdRaptorNet(nil) must be nil")
	}
}

// TestAuthdRaptorNetHarness is the reverse-engineering harness: it runs a real
// raptor title with a Recorder installed so the network ordinals it polls can
// be observed. Point AUTHD_RE_ZIP at a game .zip to run it.
func TestAuthdRaptorNetHarness(t *testing.T) {
	path := os.Getenv("AUTHD_RE_ZIP")
	if path == "" {
		t.Skip("AUTHD_RE_ZIP not set")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rec := authd.NewRecorder(nil)
	factory := application.NewFactory()
	factory.RunBudget = application.DefaultHandsetRunBudget
	factory.FrameRunBudget = application.DefaultHandsetRunBudget
	factory.RaptorNet = AuthdRaptorNet(rec)

	created, err := factory.Create(context.Background(), machinecore.Source{
		Name: filepath.Base(path), ReaderAt: bytes.NewReader(data), Size: int64(len(data)),
	})
	if err != nil {
		t.Fatal(err)
	}
	machine := created
	t.Cleanup(func() { _ = machine.Close() })
	if err := machine.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	press := os.Getenv("AUTHD_RE_PRESS")
	if press == "" {
		press = "num1"
	}
	for frame := 0; frame < 600; frame++ {
		if frame > 0 && frame%70 == 0 {
			for _, p := range []bool{true, false} {
				_ = machine.QueueInput(machinecore.InputEvent{Control: press, Pressed: p})
			}
		}
		if err := machine.StepFrame(context.Background()); err != nil {
			t.Fatalf("frame %d: %v", frame, err)
		}
	}
	events := rec.Events()
	t.Logf("recorded %d network-ordinal calls", len(events))
	seen := map[uint32]int{}
	for _, e := range events {
		seen[e.Ordinal]++
	}
	for ord, count := range seen {
		t.Logf("  ordinal %d: %d calls", ord, count)
	}
	if len(events) > 0 {
		last := events[len(events)-1]
		t.Logf("  last: ordinal=%d args=%08x", last.Ordinal, last.Args)
	}
}
