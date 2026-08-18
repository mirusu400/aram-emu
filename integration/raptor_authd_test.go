package integration

import (
	"bytes"
	"context"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

// runRaptorWithBackend loads a raptor title, installs the given netauth.Backend,
// and steps frames (pressing AUTHD_RE_PRESS every 70 frames) so the auth screen
// runs. It returns the machine for post-run inspection.
func runRaptorWithBackend(t *testing.T, path string, backend netauth.Backend) *application.Machine {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	factory := application.NewFactory()
	factory.RunBudget = application.DefaultHandsetRunBudget
	factory.FrameRunBudget = application.DefaultHandsetRunBudget
	factory.RaptorNet = backend

	created, err := factory.Create(context.Background(), machinecore.Source{
		Name: filepath.Base(path), ReaderAt: bytes.NewReader(data), Size: int64(len(data)),
	})
	if err != nil {
		t.Fatal(err)
	}
	machine := created.(*application.Machine)
	t.Cleanup(func() { _ = machine.Close() })
	if err := machine.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	press := os.Getenv("AUTHD_RE_PRESS")
	if press == "" {
		press = "num1"
	}
	frames := 600
	if v := os.Getenv("AUTHD_RE_FRAMES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			frames = n
		}
	}
	for frame := 0; frame < frames; frame++ {
		if frame > 0 && frame%70 == 0 {
			for _, p := range []bool{true, false} {
				_ = machine.QueueInput(machinecore.InputEvent{Control: press, Pressed: p})
			}
		}
		if err := machine.StepFrame(context.Background()); err != nil {
			t.Fatalf("frame %d: %v", frame, err)
		}
	}
	return machine
}

// TestAuthdRaptorNetHarness runs a real raptor title with a Recorder installed
// so the network ordinals it polls can be observed. Set AUTHD_RE_ZIP.
func TestAuthdRaptorNetHarness(t *testing.T) {
	path := os.Getenv("AUTHD_RE_ZIP")
	if path == "" {
		t.Skip("AUTHD_RE_ZIP not set")
	}
	rec := authd.NewRecorder(nil)
	runRaptorWithBackend(t, path, AuthdRaptorNet(rec))
	events := rec.Events()
	t.Logf("recorded %d network-ordinal calls", len(events))
	seen := map[uint32]int{}
	for _, e := range events {
		seen[e.Ordinal]++
	}
	for ord, count := range seen {
		t.Logf("  ordinal %d: %d calls", ord, count)
	}
}

// reObserver is an experimental netauth.Backend used to reverse-engineer the
// LGT auth handshake: on each 106/238 call it dumps the session state so the
// progression past "서버 접속중" can be observed. It declines so the default
// applies.
type reObserver struct {
	t         *testing.T
	sessPtr   uint32
	calls     int
	lastState int32
	lastEv    string
}

func (o *reObserver) Handle(call netauth.Call, mem netauth.Memory) (uint32, bool) {
	o.calls++
	if o.calls == 1 && os.Getenv("AUTHD_RE_SCAN") != "" {
		// Scan the heap for objects whose +0x574 holds -300 (the connecting
		// state) to find the ACTIVE auth object (the one at *0x1400058 is dormant).
		base, _ := parseHex(os.Getenv("AUTHD_RE_SCAN"))
		hits := 0
		for page := uint32(0); page < 0x100000; page += 0x1000 {
			chunk, err := mem.ReadBytes(base+page, 0x1000)
			if err != nil {
				continue
			}
			for i := 0; i+4 <= len(chunk); i += 4 {
				w := uint32(chunk[i]) | uint32(chunk[i+1])<<8 | uint32(chunk[i+2])<<16 | uint32(chunk[i+3])<<24
				if w == 0xFFFFFED4 { // -300
					o.t.Logf("  -300 at 0x%08x (obj base ~0x%08x)", base+page+uint32(i), base+page+uint32(i)-0x574)
					hits++
				}
			}
			if hits >= 30 {
				break
			}
		}
		o.t.Logf("scan: %d words == -300 in [0x%08x, +0x100000)", hits, base)
	}
	if o.calls <= 2 && call.SP != 0 {
		// Walk the guest stack: log words that look like code return addresses
		// (thumb, in the 0x1000..0x997d4 text range) to reconstruct the caller
		// chain that reaches the 106/238 poll.
		var chain []string
		for off := uint32(0); off < 0x200; off += 4 {
			w, err := mem.ReadU32(call.SP + off)
			if err != nil {
				break
			}
			if w >= 0x1001 && w < 0x997d4 && w&1 == 1 {
				chain = append(chain, formatHex(w))
				if len(chain) >= 24 {
					break
				}
			}
		}
		o.t.Logf("call#%d SP=0x%08x LR=0x%08x stack-code: %s", o.calls, call.SP, call.LR, strings.Join(chain, " "))
	}
	session, _ := mem.ReadU32(o.sessPtr)
	if session != 0 {
		state, _ := mem.ReadU32(session + 0x574)
		ev, _ := mem.ReadBytes(session+0x58E, 20)
		f588, _ := mem.ReadU32(session + 0x588)
		h56c, _ := mem.ReadU32(session + 0x56C)
		h570, _ := mem.ReadU32(session + 0x570)
		evStr := formatBytes(ev)
		b5d3, _ := mem.ReadU8(session + 0x5D3)
		b567, _ := mem.ReadU8(session + 0x567)
		key := int32(state)*1000 + int32(h56c&0xff) + int32(h570&0xff)*7 + int32(b5d3)*13 + int32(b567)*131
		if o.calls <= 8 || key != o.lastState || evStr != o.lastEv {
			o.t.Logf("call#%d ord=%d state=%d +588=%d h570=0x%08x +5D3=%d +567=%d ev=%s",
				o.calls, call.Ordinal, int32(state), int32(f588), h570, b5d3, b567, evStr)
			o.lastState = key
			o.lastEv = evStr
		}
	}
	// Experiment: forge a "connected + authorized" session so sub_5ADE0 reports
	// connected (state->300) and the event-check branch advances to state 21.
	if os.Getenv("AUTHD_RE_CONNECT") != "" {
		session, _ := mem.ReadU32(o.sessPtr)
		if session != 0 {
			_ = mem.WriteU8(session+0x5D3, 1) // event bit 69: connected
			_ = mem.WriteU8(session+0x567, 2) // >1: data ready
			_ = mem.WriteU8(session+0x58F, 1) // event bit 1
			_ = mem.WriteU8(session+0x590, 1) // event bit 2
			_ = mem.WriteU8(session+0x591, 1) // event bit 3
			_ = mem.WriteU8(session+0x592, 0) // event bit 4 clear
			_ = mem.WriteU8(session+0x589, 0) // gate flag clear
			if v := os.Getenv("AUTHD_RE_STATE"); v != "" {
				n, _ := parseHex(v)
				_ = mem.WriteU32(session+0x574, n) // force auth state
			}
		}
	}
	return 0, false
}

func formatBytes(b []byte) string {
	const hexdigits = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, v := range b {
		out = append(out, hexdigits[v>>4], hexdigits[v&0xf])
	}
	return string(out)
}

func parseHex(s string) (uint32, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "0x")
	n, err := strconv.ParseUint(s, 16, 32)
	return uint32(n), err
}

// TestAuthdREObserve runs a raptor title with the reObserver to watch the auth
// session state. Env: AUTHD_RE_ZIP, AUTHD_RE_SESSPTR (default 0x1400058).
func TestAuthdREObserve(t *testing.T) {
	path := os.Getenv("AUTHD_RE_ZIP")
	if path == "" {
		t.Skip("AUTHD_RE_ZIP not set")
	}
	sessPtr := uint32(0x1400058)
	if v := os.Getenv("AUTHD_RE_SESSPTR"); v != "" {
		if n, err := parseHex(v); err == nil {
			sessPtr = n
		}
	}
	obs := &reObserver{t: t, sessPtr: sessPtr, lastState: 1 << 30}
	machine := runRaptorWithBackend(t, path, obs)
	t.Logf("total network calls: %d", obs.calls)

	img := machine.Framebuffer()
	b := img.Bounds()
	nonBlack := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if r|g|bl != 0 {
				nonBlack++
			}
		}
	}
	t.Logf("final frame: nonblack=%d / %d", nonBlack, b.Dx()*b.Dy())
	if out := os.Getenv("AUTHD_RE_PNG"); out != "" {
		f, err := os.Create(out)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
		t.Logf("wrote %s", out)
	}
}

func formatHex(v uint32) string {
	const d = "0123456789abcdef"
	var b [10]byte
	b[0], b[1] = '0', 'x'
	for i := 0; i < 8; i++ {
		b[2+i] = d[(v>>(28-4*i))&0xf]
	}
	return string(b[:])
}
