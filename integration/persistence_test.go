//go:build !js || !wasm

package integration

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	aramcore "github.com/mirusu400/aram-core/core"
	"github.com/mirusu400/aram-frontend/frontend"
)

type persistenceMachine struct {
	*saveStubMachine
	exportErr   error
	closed      bool
	loadedState []byte
}

func (machine *persistenceMachine) ExportSaveData() ([]byte, error) {
	if machine.exportErr != nil {
		return nil, machine.exportErr
	}
	return machine.saveStubMachine.ExportSaveData()
}

func (machine *persistenceMachine) Close() error { machine.closed = true; return nil }

func (machine *persistenceMachine) LoadState(reader io.Reader) error {
	data, err := io.ReadAll(reader)
	machine.loadedState = data
	return err
}

type persistenceFactory struct{ machine aramcore.Machine }

func (factory persistenceFactory) Create(context.Context, aramcore.Source) (aramcore.Machine, error) {
	return factory.machine, nil
}

func TestCloseRetainsSessionAfterSaveFailureForRetry(t *testing.T) {
	for _, failure := range []string{"export", "write"} {
		t.Run(failure, func(t *testing.T) {
			machine := &persistenceMachine{saveStubMachine: &saveStubMachine{data: []byte("new progress")}}
			backend := &Backend{stateRoot: t.TempDir(), machine: machine, input: frontend.InputInfo{SHA256: saveHashA}, runRequested: true}
			file, err := os.CreateTemp(t.TempDir(), "source")
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			backend.sourceFile = file
			blocker := filepath.Join(backend.stateRoot, saveHashA)
			if failure == "export" {
				machine.exportErr = errors.New("synthetic export failure")
			} else if err := os.WriteFile(blocker, []byte("blocker"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := backend.Close(); err == nil {
				t.Fatal("failed save closed successfully")
			}
			if backend.machine != machine || machine.closed || backend.input.SHA256 != saveHashA || backend.sourceFile != file {
				t.Fatal("failed save discarded session or identity")
			}
			if _, err := file.Stat(); err != nil {
				t.Fatalf("source closed before save could be retried: %v", err)
			}
			machine.exportErr = nil
			if failure == "write" {
				if err := os.Remove(blocker); err != nil {
					t.Fatal(err)
				}
			}
			if err := backend.Close(); err != nil {
				t.Fatalf("retry: %v", err)
			}
			data, err := os.ReadFile(filepath.Join(backend.stateRoot, saveHashA, "savedata.bin"))
			if err != nil || !bytes.Equal(data, machine.data) {
				t.Fatalf("retry save = %q, %v", data, err)
			}
			if !machine.closed || backend.machine != nil {
				t.Fatal("successful retry did not close session")
			}
		})
	}
}

func TestOpenRetainsOldSessionWhenItsSaveFails(t *testing.T) {
	old := &persistenceMachine{saveStubMachine: &saveStubMachine{data: []byte("old progress")}, exportErr: errors.New("synthetic export failure")}
	next := &persistenceMachine{saveStubMachine: &saveStubMachine{}}
	backend := &Backend{stateRoot: t.TempDir(), factory: persistenceFactory{next}, machine: old, input: frontend.InputInfo{SHA256: saveHashA}}
	if _, err := backend.Open(context.Background(), frontend.OpenRequest{Data: syntheticEADS(), DisplayName: "synthetic.dat"}); err == nil {
		t.Fatal("replacement succeeded after old save failed")
	}
	if backend.machine != old || old.closed || backend.input.SHA256 != saveHashA || !next.closed {
		t.Fatal("replacement did not preserve old session and dispose new candidate")
	}
}

func TestReadSaveDataRecoversInterruptedReplacement(t *testing.T) {
	backend := &Backend{stateRoot: t.TempDir()}
	path, err := backend.saveDataFileFor(saveHashA)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".bak", []byte("previous progress"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := backend.readSaveData(saveHashA)
	if err != nil || string(got) != "previous progress" {
		t.Fatalf("recovered save = %q, %v", got, err)
	}
}

func TestReplaceFailurePreservesInterruptedBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save")
	if err := os.WriteFile(path+".bak", []byte("previous progress"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := replaceFileCrashSafely(path+".missing", path); err == nil {
		t.Fatal("missing temporary was accepted")
	}
	data, targetErr := os.ReadFile(path)
	if targetErr != nil {
		data, targetErr = os.ReadFile(path + ".bak")
	}
	if targetErr != nil || string(data) != "previous progress" {
		t.Fatalf("original save lost: %q, %v", data, targetErr)
	}
}

func TestLoadStateRecoversInterruptedReplacement(t *testing.T) {
	machine := &persistenceMachine{saveStubMachine: &saveStubMachine{}}
	backend := &Backend{stateRoot: t.TempDir(), machine: machine, input: frontend.InputInfo{SHA256: saveHashA}}
	if err := backend.saveState(0); err != nil {
		t.Fatal(err)
	}
	path, err := backend.statePath(0)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, path+".bak"); err != nil {
		t.Fatal(err)
	}
	if err := backend.loadState(0); err != nil {
		t.Fatal(err)
	}
	if string(machine.loadedState) != "stub state" {
		t.Fatalf("restored state = %q", machine.loadedState)
	}
}
