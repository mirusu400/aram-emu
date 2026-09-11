package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mirusu400/aram-frontend/frontend"
)

func TestOrdinaryOpenJ2MEIdentityAndLifecycle(t *testing.T) {
	ctx := context.Background()
	for _, wrapped := range []bool{false, true} {
		data := syntheticJ2ME(t, wrapped)
		for _, inMemory := range []bool{false, true} {
			for _, profile := range []string{"", "j2me-1.0/lgt/generic"} {
				t.Run(fmt.Sprintf("zip=%t/bytes=%t/profile=%s", wrapped, inMemory, profile), func(t *testing.T) {
					backend := NewBackend(nil)
					if err := backend.ConfigureStateRoot(t.TempDir()); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = backend.Close() })
					name := "synthetic.JAR"
					if wrapped {
						name = "synthetic.ZIP"
					}
					request := frontend.OpenRequest{DisplayName: name, ProfileID: profile}
					if inMemory {
						request.Data = data
					} else {
						request.Path = filepath.Join(t.TempDir(), name)
						if err := os.WriteFile(request.Path, data, 0o600); err != nil {
							t.Fatal(err)
						}
					}
					var stages []frontend.OpenStage
					info, err := backend.OpenWithProgress(ctx, request, func(stage frontend.OpenStage) {
						stages = append(stages, stage)
					})
					if err != nil {
						t.Fatal(err)
					}
					wantProfile := profile
					if wantProfile == "" {
						wantProfile = "j2me-1.0/generic/generic"
					}
					if info.DisplayName != name || info.Format != "j2me" || info.ProfileID != wantProfile ||
						info.Size != int64(len(data)) || info.SHA256 != fmt.Sprintf("%x", sha256.Sum256(data)) {
						t.Fatalf("Java input identity = %+v", info)
					}
					if !slices.Equal(stages, []frontend.OpenStage{frontend.OpenStageInspecting, frontend.OpenStageLoading}) {
						t.Fatalf("ordinary open stages = %v", stages)
					}
					if backend.State() != frontend.StateReady {
						t.Fatalf("state after open = %s", backend.State())
					}
					for _, command := range []frontend.BackendCommand{
						frontend.CommandStart, frontend.CommandFrame, frontend.CommandReset,
						frontend.CommandSaveState, frontend.CommandLoadState,
					} {
						if capability := backend.Capability(command); !capability.Supported {
							t.Errorf("%s capability = %+v", command, capability)
						}
					}
					for _, command := range []frontend.BackendCommand{frontend.CommandRewind, frontend.CommandFastForward} {
						if capability := backend.Capability(command); capability.Supported || capability.Reason == "" {
							t.Errorf("unsupported %s must retain an explanation: %+v", command, capability)
						}
					}
					for _, command := range []frontend.BackendCommand{
						frontend.CommandStart, frontend.CommandSaveState,
						frontend.CommandLoadState, frontend.CommandPauseResume,
						frontend.CommandFrame,
					} {
						if err := backend.Execute(ctx, command); err != nil {
							t.Fatalf("%s: %v", command, err)
						}
					}
					if backend.State() != frontend.StatePaused {
						t.Fatalf("manual frame changed pause intent: %s", backend.State())
					}
					if err := backend.QueueInput(frontend.InputEvent{Control: "ok", Pressed: true}); err != nil {
						t.Fatal(err)
					}
					if err := backend.Execute(ctx, frontend.CommandFrame); err != nil {
						t.Fatal(err)
					}
					snapshot, ok := backend.CoreDebugSnapshot(1)
					if !ok || snapshot.Runtime != "j2me" || snapshot.SKVM == nil ||
						snapshot.SKVM.MainClass != "ContractMIDlet" || !snapshot.SKVM.Started || snapshot.SKVM.Instructions == 0 {
						t.Fatalf("shared Java execution snapshot = %+v", snapshot)
					}
					if diagnostics := backend.Diagnostics(); diagnostics.Java == nil || diagnostics.Java.HasDisplay {
						t.Fatalf("return-only MIDlet must not report a guest display: %+v", diagnostics.Java)
					}
					debugger, err := backend.ToolSnapshot(ctx, frontend.ToolDebugger)
					if err != nil {
						t.Fatal(err)
					}
					text := strings.Join(debugger.Lines, "\n")
					if !strings.Contains(text, "j2me") || !strings.Contains(text, "ContractMIDlet") || strings.Contains(text, "(ARM)") {
						t.Fatalf("Java debugger must not pretend to be ARM: %s", text)
					}
					for _, command := range []frontend.BackendCommand{frontend.CommandStop, frontend.CommandReset} {
						if err := backend.Execute(ctx, command); err != nil {
							t.Fatalf("%s: %v", command, err)
						}
					}
					if backend.State() != frontend.StateReady {
						t.Fatalf("state after reset = %s", backend.State())
					}
				})
			}
		}
	}
}

// This fixture is a generated, return-only MIDlet. It verifies product routing
// and lifecycle contracts, not a rendered game screen or carrier compatibility.
func syntheticJ2ME(t *testing.T, wrapped bool) []byte {
	t.Helper()
	manifest := []byte("Manifest-Version: 1.0\r\nMIDlet-Name: Contract\r\nMIDlet-Version: 1.0\r\nMIDlet-Vendor: ARAM\r\nMIDlet-1: Contract,,ContractMIDlet\r\nMicroEdition-Configuration: CLDC-1.0\r\nMicroEdition-Profile: MIDP-1.0\r\n\r\n")
	jar := syntheticZIP(t, map[string][]byte{
		"META-INF/MANIFEST.MF": manifest,
		"ContractMIDlet.class": syntheticJ2MEClass(t),
	})
	if !wrapped {
		return jar
	}
	jad := append(bytes.Clone(manifest), []byte(fmt.Sprintf("MIDlet-Jar-URL: contract.jar\r\nMIDlet-Jar-Size: %d\r\n", len(jar)))...)
	return syntheticZIP(t, map[string][]byte{"contract.jar": jar, "contract.jad": jad})
}

func syntheticJ2MEClass(t *testing.T) []byte {
	t.Helper()
	var output bytes.Buffer
	write := func(value any) {
		t.Helper()
		if err := binary.Write(&output, binary.BigEndian, value); err != nil {
			t.Fatal(err)
		}
	}
	utf := func(value string) {
		write(uint8(1))
		write(uint16(len(value)))
		output.WriteString(value)
	}
	write(uint32(0xcafebabe))
	write([]uint16{3, 45, 14})
	utf("ContractMIDlet") // 1
	write(uint8(7))
	write(uint16(1))                        // class 2
	utf("javax/microedition/midlet/MIDlet") // 3
	write(uint8(7))
	write(uint16(3)) // superclass 4
	for _, value := range []string{"<init>", "()V", "Code", "startApp", "pauseApp", "destroyApp", "(Z)V"} {
		utf(value) // 5..11
	}
	write(uint8(12))
	write([]uint16{5, 6}) // NameAndType 12
	write(uint8(10))
	write([]uint16{4, 12}) // superclass constructor 13
	write([]uint16{0x21, 2, 4, 0, 0, 4})
	for _, method := range []struct {
		name, descriptor, locals uint16
		code                     []byte
	}{
		{5, 6, 1, []byte{0x2a, 0xb7, 0, 13, 0xb1}},
		{8, 6, 1, []byte{0xb1}},
		{9, 6, 1, []byte{0xb1}},
		{10, 11, 2, []byte{0xb1}},
	} {
		write([]uint16{1, method.name, method.descriptor, 1, 7})
		write(uint32(12 + len(method.code)))
		write([]uint16{1, method.locals})
		write(uint32(len(method.code)))
		output.Write(method.code)
		write([]uint16{0, 0})
	}
	write(uint16(0))
	return output.Bytes()
}
