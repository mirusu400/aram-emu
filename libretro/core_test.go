package libretro

import (
	"context"
	"encoding/binary"
	"testing"
)

func TestCoreLoadsRunsAndRestoresSyntheticApplication(t *testing.T) {
	core := New(Config{SaveDirectory: t.TempDir(), SerializeSize: 40 << 20})
	t.Cleanup(func() { _ = core.Close() })

	if err := core.LoadGame(context.Background(), "synthetic.dat", syntheticEADS()); err != nil {
		t.Fatal(err)
	}
	info := core.AVInfo()
	if info.Width != 240 || info.Height != 320 || info.FPS != 62.5 || info.SampleRate != 44100 {
		t.Fatalf("AV info = %+v", info)
	}

	result, err := core.Run(context.Background(), 1<<JoypadA)
	if err != nil {
		t.Fatal(err)
	}
	if result.Frame.Width != 240 || result.Frame.Height != 320 ||
		len(result.Frame.Pixels) != 240*320 {
		t.Fatalf("frame = %dx%d, pixels=%d", result.Frame.Width, result.Frame.Height, len(result.Frame.Pixels))
	}

	state := make([]byte, core.SerializeSize())
	if err := core.Serialize(state); err != nil {
		t.Fatal(err)
	}
	if err := core.Reset(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := core.Unserialize(state); err != nil {
		t.Fatal(err)
	}
	if _, err := core.Run(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
}

func TestStateIsBoundToLoadedContent(t *testing.T) {
	first := New(Config{SerializeSize: 40 << 20})
	t.Cleanup(func() { _ = first.Close() })
	if err := first.LoadGame(context.Background(), "first.dat", syntheticEADS()); err != nil {
		t.Fatal(err)
	}
	state := make([]byte, first.SerializeSize())
	if err := first.Serialize(state); err != nil {
		t.Fatal(err)
	}

	otherContent := syntheticEADS()
	otherContent[0] = 1
	second := New(Config{SerializeSize: 40 << 20})
	t.Cleanup(func() { _ = second.Close() })
	if err := second.LoadGame(context.Background(), "second.dat", otherContent); err != nil {
		t.Fatal(err)
	}
	if err := second.Unserialize(state); err == nil {
		t.Fatal("state for different content was accepted")
	}
}

func TestSerializeRejectsShortBuffer(t *testing.T) {
	core := New(Config{SerializeSize: 40 << 20})
	t.Cleanup(func() { _ = core.Close() })
	if err := core.LoadGame(context.Background(), "synthetic.dat", syntheticEADS()); err != nil {
		t.Fatal(err)
	}
	if err := core.Serialize(make([]byte, core.SerializeSize()-1)); err == nil {
		t.Fatal("short state buffer was accepted")
	}
}

func TestCoreRejectsMalformedContentWithoutKeepingAMachine(t *testing.T) {
	core := New(Config{})
	if err := core.LoadGame(context.Background(), "broken.dat", []byte("not a WIPI image")); err == nil {
		t.Fatal("malformed content was accepted")
	}
	if core.Loaded() {
		t.Fatal("failed load retained a machine")
	}
}

func TestMappedControlsCoverHandsetAndKeypadLayers(t *testing.T) {
	normal := mappedControls((1 << JoypadUp) | (1 << JoypadA) | (1 << JoypadX))
	assertControls(t, normal, "up", "ok", "soft-left")

	keypad := mappedControls((1 << JoypadSelect) | (1 << JoypadUp) | (1 << JoypadA) | (1 << JoypadR))
	assertControls(t, keypad, "num2", "num5", "num9")
}

func TestStereoPCMConvertsMonoAndKeepsStereo(t *testing.T) {
	mono := stereoPCM([]int16{1, -2, 3}, 1)
	wantMono := []int16{1, 1, -2, -2, 3, 3}
	if !equalPCM(mono, wantMono) {
		t.Fatalf("mono conversion = %v, want %v", mono, wantMono)
	}
	stereo := []int16{1, 2, 3, 4}
	if got := stereoPCM(stereo, 2); !equalPCM(got, stereo) {
		t.Fatalf("stereo conversion = %v", got)
	}
}

func assertControls(t *testing.T, got map[string]bool, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("controls = %v, want %v", got, want)
	}
	for _, control := range want {
		if !got[control] {
			t.Fatalf("controls = %v, missing %q", got, control)
		}
	}
}

func equalPCM(left, right []int16) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func syntheticEADS() []byte {
	const offset = 0x80
	data := make([]byte, offset+0x30+6)
	copy(data[offset:], "EADS")
	binary.LittleEndian.PutUint32(data[offset+4:], 1)
	binary.LittleEndian.PutUint32(data[offset+8:], 1)
	binary.LittleEndian.PutUint32(data[offset+12:], 0x02000000)
	binary.LittleEndian.PutUint32(data[offset+16:], 6)
	binary.LittleEndian.PutUint32(data[offset+20:], 0x03000000)
	binary.LittleEndian.PutUint32(data[offset+24:], 0x1000)
	copy(data[offset+0x20:], "SyntheticEADS")
	copy(data[offset+0x30:], []byte{0x00, 0xb5, 0x00, 0xbe, 0xfe, 0xe7})
	return data
}
