package libretro

import (
	"context"
	"errors"
	"image"
	"os"
	"path/filepath"
	"testing"

	aramcore "github.com/mirusu400/aram-core/core"
)

type persistenceMachine struct {
	aramcore.Machine
	data      []byte
	exportErr error
	closed    bool
	state     aramcore.State
}

func (machine *persistenceMachine) State() aramcore.State {
	if machine.state == aramcore.StateEmpty {
		return aramcore.StatePaused
	}
	return machine.state
}
func (machine *persistenceMachine) Pause() error { machine.state = aramcore.StatePaused; return nil }
func (machine *persistenceMachine) Start(context.Context) error {
	machine.state = aramcore.StateRunning
	return nil
}
func (machine *persistenceMachine) Framebuffer() image.Image {
	return image.NewRGBA(image.Rect(0, 0, 1, 1))
}
func (machine *persistenceMachine) DrainAudio() aramcore.AudioChunk { return aramcore.AudioChunk{} }
func (machine *persistenceMachine) ExportSaveData() ([]byte, error) {
	return machine.data, machine.exportErr
}
func (machine *persistenceMachine) ImportSaveData(data []byte) error {
	machine.data = append([]byte(nil), data...)
	return nil
}
func (machine *persistenceMachine) Close() error { machine.closed = true; return nil }

func TestUnloadRetainsSessionAfterSaveFailureForRetry(t *testing.T) {
	for _, failure := range []string{"export", "write"} {
		t.Run(failure, func(t *testing.T) {
			machine := &persistenceMachine{data: []byte("new progress"), state: aramcore.StateRunning}
			core := New(Config{SaveDirectory: t.TempDir()})
			core.machine, core.sourceSHA256 = machine, "synthetic"
			core.started = true
			blocker := filepath.Join(core.saveDirectory, "aram")
			if failure == "export" {
				machine.exportErr = errors.New("synthetic export failure")
			} else if err := os.WriteFile(blocker, []byte("blocker"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := core.Unload(); err == nil {
				t.Fatal("failed save unloaded successfully")
			}
			if core.machine != machine || machine.closed || core.sourceSHA256 != "synthetic" {
				t.Fatal("failed save discarded session")
			}
			if _, err := core.Run(context.Background(), 0); err != nil {
				t.Fatalf("resume after failed unload: %v", err)
			}
			if machine.State() != aramcore.StateRunning {
				t.Fatal("failed unload left the retained game unable to resume")
			}
			machine.exportErr = nil
			if failure == "write" {
				if err := os.Remove(blocker); err != nil {
					t.Fatal(err)
				}
			}
			path := core.savePath()
			if err := core.Unload(); err != nil {
				t.Fatalf("retry: %v", err)
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != "new progress" {
				t.Fatalf("retry save = %q, %v", data, err)
			}
			if !machine.closed || core.Loaded() {
				t.Fatal("successful retry did not unload")
			}
		})
	}
}

func TestLoadPersistentDataRecoversInterruptedReplacement(t *testing.T) {
	machine := &persistenceMachine{}
	core := New(Config{SaveDirectory: t.TempDir()})
	core.machine, core.sourceSHA256 = machine, "synthetic"
	path := core.savePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".bak", []byte("previous progress"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := core.loadPersistentData(); err != nil {
		t.Fatal(err)
	}
	if string(machine.data) != "previous progress" {
		t.Fatalf("restored save = %q", machine.data)
	}
}
