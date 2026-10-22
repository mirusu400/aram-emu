package systemintegration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mirusu400/aram-core/firmwareset"
	"github.com/mirusu400/aram-core/systemmachine"
	"github.com/mirusu400/aram-frontend/frontend"
)

func openFakeStateBackend(t *testing.T, configure func(*Options)) (*Backend, *fakeSystemMachine, string) {
	t.Helper()
	firmwareDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(firmwareDirectory, "phone.dat"), []byte("firmware"), 0o600); err != nil {
		t.Fatal(err)
	}
	stateRoot := filepath.Join(t.TempDir(), "states")
	machine := newFakeSystemMachine()
	options := Options{
		InstructionsPerFrame:     25,
		MinimumInputInstructions: 50,
		CPUBackendMode:           systemmachine.CPUBackendPrecise,
		StateRoot:                stateRoot,
		DisableMediaPersistence:  true,
		newMachine: func(firmwareset.Set, systemmachine.Options) (systemMachine, error) {
			return machine, nil
		},
	}
	if configure != nil {
		configure(&options)
	}
	backend := NewBackend(options)
	t.Cleanup(func() { _ = backend.Close() })
	if _, err := backend.Open(context.Background(), frontend.OpenRequest{Path: firmwareDirectory}); err != nil {
		t.Fatal(err)
	}
	return backend, machine, stateRoot
}

func TestConfigureStateRootCreatesAbsoluteRootAndRejectsOpenMachine(t *testing.T) {
	backend := NewBackend(Options{CPUBackendMode: systemmachine.CPUBackendPrecise})
	if err := backend.ConfigureStateRoot("  "); err == nil {
		t.Fatal("empty state root was accepted")
	}
	root := filepath.Join(t.TempDir(), "isolated")
	if err := backend.ConfigureStateRoot(root); err != nil {
		t.Fatal(err)
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	if backend.stateRoot != absolute {
		t.Fatalf("state root = %q, want %q", backend.stateRoot, absolute)
	}
	if info, err := os.Stat(absolute); err != nil || !info.IsDir() {
		t.Fatalf("configured state root is not a directory: %v", err)
	}

	opened, _, _ := openFakeStateBackend(t, nil)
	if err := opened.ConfigureStateRoot(t.TempDir()); err == nil {
		t.Fatal("state root changed while a machine was open")
	}
}

func TestWholePhoneSaveAndLoadStateWhileRunning(t *testing.T) {
	backend, machine, stateRoot := openFakeStateBackend(t, nil)
	if err := backend.Execute(context.Background(), frontend.CommandStart); err != nil {
		t.Fatal(err)
	}
	if err := backend.RunFrame(context.Background()); err != nil {
		t.Fatal(err)
	}
	saved := machine.position
	if capability := backend.Capability(frontend.CommandSaveState); !capability.Supported {
		t.Fatalf("save-state capability = %+v", capability)
	}
	if capability := backend.Capability(frontend.CommandLoadState); !capability.Supported {
		t.Fatalf("load-state capability = %+v", capability)
	}
	if err := backend.ExecuteCommand(context.Background(), frontend.CommandRequest{
		Command: frontend.CommandSaveState,
		Slot:    4,
	}); err != nil {
		t.Fatal(err)
	}
	if err := backend.RunFrame(context.Background()); err != nil {
		t.Fatal(err)
	}
	if machine.position == saved {
		t.Fatal("machine did not advance after saving")
	}
	if err := backend.ExecuteCommand(context.Background(), frontend.CommandRequest{
		Command: frontend.CommandLoadState,
		Slot:    4,
	}); err != nil {
		t.Fatal(err)
	}
	if machine.position != saved {
		t.Fatalf("loaded position = %+v, want %+v", machine.position, saved)
	}
	if backend.State() != frontend.StateRunning {
		t.Fatalf("state after running load = %s, want running", backend.State())
	}
	if machine.snapshotLoads != 1 {
		t.Fatalf("snapshot loads = %d, want 1", machine.snapshotLoads)
	}
	path := filepath.Join(stateRoot, backend.input.SHA256, "slot-4.aramstate")
	if info, err := os.Stat(path); err != nil || info.Size() == 0 {
		t.Fatalf("state slot was not persisted: %v", err)
	}
}

func TestWholePhoneStateRejectsCorruptionWithoutMutatingMachine(t *testing.T) {
	backend, machine, _ := openFakeStateBackend(t, nil)
	if err := backend.Execute(context.Background(), frontend.CommandStart); err != nil {
		t.Fatal(err)
	}
	if err := backend.RunFrame(context.Background()); err != nil {
		t.Fatal(err)
	}
	request := frontend.CommandRequest{Command: frontend.CommandSaveState, Slot: 2}
	if err := backend.ExecuteCommand(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	path, err := backend.statePath(2)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)-1] ^= 0xff
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := backend.RunFrame(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := machine.position
	err = backend.ExecuteCommand(context.Background(), frontend.CommandRequest{
		Command: frontend.CommandLoadState,
		Slot:    2,
	})
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("corrupt load error = %v", err)
	}
	if machine.position != before || machine.snapshotLoads != 0 {
		t.Fatalf("corrupt state mutated machine: position=%+v loads=%d", machine.position, machine.snapshotLoads)
	}
	if backend.State() != frontend.StateRunning {
		t.Fatalf("corrupt load changed backend state to %s", backend.State())
	}
}

func TestWholePhoneStateValidatesFirmwareIdentityBeforeRestore(t *testing.T) {
	backend, machine, _ := openFakeStateBackend(t, nil)
	if err := backend.Execute(context.Background(), frontend.CommandStart); err != nil {
		t.Fatal(err)
	}
	if err := backend.RunFrame(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := backend.ExecuteCommand(context.Background(), frontend.CommandRequest{
		Command: frontend.CommandSaveState,
		Slot:    1,
	}); err != nil {
		t.Fatal(err)
	}
	path, err := backend.statePath(1)
	if err != nil {
		t.Fatal(err)
	}
	header, snapshot, err := readSystemStateFile(path)
	if err != nil {
		t.Fatal(err)
	}
	header.ProfileID = "samsung.sch-w860.da06"
	if err := writeSystemStateFile(path, header, snapshot); err != nil {
		t.Fatal(err)
	}
	before := machine.position
	err = backend.ExecuteCommand(context.Background(), frontend.CommandRequest{
		Command: frontend.CommandLoadState,
		Slot:    1,
	})
	if err == nil || !strings.Contains(err.Error(), "different firmware profile") {
		t.Fatalf("profile mismatch error = %v", err)
	}
	if machine.position != before || machine.snapshotLoads != 0 {
		t.Fatalf("profile mismatch mutated machine: position=%+v loads=%d", machine.position, machine.snapshotLoads)
	}
}

func TestWholePhoneRewindRestoresBoundedCheckpointsAndKeepsRunning(t *testing.T) {
	backend, machine, _ := openFakeStateBackend(t, func(options *Options) {
		options.RewindIntervalFrames = 1
		options.RewindCapacity = 3
		options.RewindMemoryLimitBytes = 1024
	})
	if err := backend.Execute(context.Background(), frontend.CommandStart); err != nil {
		t.Fatal(err)
	}
	for range 4 {
		if err := backend.RunFrame(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(backend.rewindHistory); got != 3 {
		t.Fatalf("rewind history length = %d, want bounded length 3", got)
	}
	if capability := backend.Capability(frontend.CommandRewind); !capability.Supported {
		t.Fatalf("rewind capability = %+v", capability)
	}
	if err := backend.Execute(context.Background(), frontend.CommandRewind); err != nil {
		t.Fatal(err)
	}
	if machine.position.Instructions != 75 {
		t.Fatalf("first rewind instructions = %d, want 75", machine.position.Instructions)
	}
	if err := backend.Execute(context.Background(), frontend.CommandRewind); err != nil {
		t.Fatal(err)
	}
	if machine.position.Instructions != 50 {
		t.Fatalf("second rewind instructions = %d, want 50", machine.position.Instructions)
	}
	if backend.State() != frontend.StateRunning {
		t.Fatalf("state after rewind = %s, want running", backend.State())
	}

	// Advancing after a rewind anchors the new branch so it can itself be undone.
	if err := backend.RunFrame(context.Background()); err != nil {
		t.Fatal(err)
	}
	if machine.position.Instructions != 75 {
		t.Fatalf("branched instructions = %d, want 75", machine.position.Instructions)
	}
	if err := backend.Execute(context.Background(), frontend.CommandRewind); err != nil {
		t.Fatal(err)
	}
	if machine.position.Instructions != 50 {
		t.Fatalf("branch rewind instructions = %d, want 50", machine.position.Instructions)
	}
}

func TestWholePhoneRewindLimitDisablesHistoryWithoutStoppingMachine(t *testing.T) {
	backend, machine, _ := openFakeStateBackend(t, func(options *Options) {
		options.RewindIntervalFrames = 1
		options.RewindMemoryLimitBytes = 1
	})
	if err := backend.Execute(context.Background(), frontend.CommandStart); err != nil {
		t.Fatal(err)
	}
	if err := backend.RunFrame(context.Background()); err != nil {
		t.Fatal(err)
	}
	if machine.position.Instructions != 25 || backend.State() != frontend.StateRunning {
		t.Fatalf("rewind limit interrupted machine: position=%+v state=%s", machine.position, backend.State())
	}
	capability := backend.Capability(frontend.CommandRewind)
	if capability.Supported || !strings.Contains(capability.Reason, "above the 1-byte rewind limit") {
		t.Fatalf("rewind capability after limit = %+v", capability)
	}
	if err := backend.Execute(context.Background(), frontend.CommandRewind); err == nil || !strings.Contains(err.Error(), "above the 1-byte rewind limit") {
		t.Fatalf("rewind without history error = %v", err)
	}
}

func TestReferenceWholePhoneStateRoundTrip(t *testing.T) {
	reference := os.Getenv("ARAM_REFERENCE_REPO")
	if reference == "" {
		t.Skip("ARAM_REFERENCE_REPO is not set")
	}
	directory := filepath.Join(reference, "SCH-W380_DL21")
	if _, err := os.Stat(directory); err != nil {
		t.Skipf("reference firmware directory is unavailable: %v", err)
	}
	backend := NewBackend(Options{
		InstructionsPerFrame:     32,
		MinimumInputInstructions: 32,
		CPUBackendMode:           systemmachine.CPUBackendPrecise,
		StateRoot:                filepath.Join(t.TempDir(), "states"),
		DisableMediaPersistence:  true,
	})
	t.Cleanup(func() { _ = backend.Close() })
	if _, err := backend.Open(context.Background(), frontend.OpenRequest{Path: directory}); err != nil {
		t.Fatal(err)
	}
	machine := backend.currentMachine()
	if machine == nil {
		t.Fatal("reference firmware did not create a system machine")
	}
	want := machine.Position()
	if err := backend.ExecuteCommand(context.Background(), frontend.CommandRequest{
		Command: frontend.CommandSaveState,
		Slot:    0,
	}); err != nil {
		t.Fatal(err)
	}
	if err := backend.Execute(context.Background(), frontend.CommandFrame); err != nil {
		t.Fatal(err)
	}
	if got := machine.Position(); got == want {
		t.Fatalf("reference machine did not advance: %+v", got)
	}
	if capability := backend.Capability(frontend.CommandRewind); !capability.Supported {
		t.Fatalf("reference rewind capability = %+v", capability)
	}
	if err := backend.Execute(context.Background(), frontend.CommandRewind); err != nil {
		t.Fatal(err)
	}
	if got := machine.Position(); got != want {
		t.Fatalf("reference rewound position = %+v, want %+v", got, want)
	}
	if err := backend.Execute(context.Background(), frontend.CommandFrame); err != nil {
		t.Fatal(err)
	}
	if err := backend.ExecuteCommand(context.Background(), frontend.CommandRequest{
		Command: frontend.CommandLoadState,
		Slot:    0,
	}); err != nil {
		t.Fatal(err)
	}
	if got := machine.Position(); got != want {
		t.Fatalf("reference restored position = %+v, want %+v", got, want)
	}
}
