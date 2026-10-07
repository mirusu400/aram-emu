package systemintegration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mirusu400/aram-core/firmwareset"
	"github.com/mirusu400/aram-core/systemmachine"
	"github.com/mirusu400/aram-frontend/frontend"
)

func TestCloseRetainsSessionAfterMediaSaveFailureForRetry(t *testing.T) {
	machine := newFakeSystemMachine()
	root := filepath.Join(t.TempDir(), "media")
	if err := os.WriteFile(root, []byte("blocker"), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := NewBackend(Options{MediaRoot: root})
	backend.machine, backend.contentID = machine, "synthetic"
	backend.input = frontend.InputInfo{SHA256: "synthetic"}
	backend.state = frontend.StatePaused
	if err := backend.Close(); err == nil {
		t.Fatal("failed media save closed successfully")
	}
	if backend.machine != machine || machine.closed || backend.contentID != "synthetic" || backend.input.SHA256 != "synthetic" {
		t.Fatal("failed media save discarded session")
	}
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("retry: %v", err)
	}
	got, err := readMediaFile(filepath.Join(root, "synthetic.arammedia"))
	if err != nil || string(got.Flash) != string(machine.media.Flash) {
		t.Fatalf("retry media = %+v, %v", got, err)
	}
	if !machine.closed || backend.machine != nil {
		t.Fatal("successful retry did not close")
	}
}

func TestOpenWithUnavailableMediaRootRetainsSessionUntilSaveRetry(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "phone.wbt"), []byte("boot"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "media")
	if err := os.WriteFile(root, []byte("blocker"), 0o600); err != nil {
		t.Fatal(err)
	}
	machine := newFakeSystemMachine()
	backend := NewBackend(Options{MediaRoot: root, CPUBackendMode: systemmachine.CPUBackendPrecise, newMachine: func(firmwareset.Set, systemmachine.Options) (systemMachine, error) { return machine, nil }})
	info, err := backend.Open(context.Background(), frontend.OpenRequest{Path: directory})
	if err != nil {
		t.Fatal(err)
	}
	contentID := backend.contentID
	if backend.mediaWarning == "" {
		t.Fatal("unavailable media storage did not warn")
	}
	if err := backend.Close(); err == nil {
		t.Fatal("unavailable storage silently discarded unsaved session")
	}
	if backend.machine != machine || machine.closed || backend.contentID != contentID || backend.input.SHA256 != info.SHA256 {
		t.Fatal("unavailable storage discarded session or identity")
	}
	for _, file := range backend.firmwareFiles {
		if _, err := file.Stat(); err != nil {
			t.Fatalf("firmware closed before save retry: %v", err)
		}
	}
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("save retry after restoring storage: %v", err)
	}
	got, err := readMediaFile(filepath.Join(root, contentID+".arammedia"))
	if err != nil || string(got.Flash) != string(machine.media.Flash) || string(got.NAND) != string(machine.media.NAND) {
		t.Fatalf("retried media = %+v, %v", got, err)
	}
	if !machine.closed || backend.machine != nil {
		t.Fatal("successful retry did not close session")
	}
}

func TestSaveRetryPreservesMediaDiscoveredAfterStorageRecovers(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "phone.wbt"), []byte("boot"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "media")
	if err := os.WriteFile(root, []byte("blocker"), 0o600); err != nil {
		t.Fatal(err)
	}
	machine := newFakeSystemMachine()
	backend := NewBackend(Options{MediaRoot: root, CPUBackendMode: systemmachine.CPUBackendPrecise, newMachine: func(firmwareset.Set, systemmachine.Options) (systemMachine, error) { return machine, nil }})
	if _, err := backend.Open(context.Background(), frontend.OpenRequest{Path: directory}); err != nil {
		t.Fatal(err)
	}
	contentID := backend.contentID
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, contentID+".arammedia")
	want := newFakeSystemMachine().media
	want.Flash = []byte("previous progress from recovered storage")
	if err := writeMediaFile(path, want); err != nil {
		t.Fatal(err)
	}
	if err := backend.persistCurrentMedia(machine); err != nil {
		t.Fatal(err)
	}
	if !backend.mediaRestoreFailed || backend.mediaWarning == "" {
		t.Fatal("newly discovered media was not protected with a warning")
	}
	if err := backend.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := readMediaFile(path)
	if err != nil || string(got.Flash) != string(want.Flash) {
		t.Fatalf("newly discovered media was overwritten: %+v, %v", got, err)
	}
}

func TestReadMediaRecoversInterruptedReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "phone.arammedia")
	want := newFakeSystemMachine().media
	if err := writeMediaFile(path+".bak", want); err != nil {
		t.Fatal(err)
	}
	got, err := readMediaFile(path)
	if err != nil || string(got.Flash) != string(want.Flash) {
		t.Fatalf("recovered media = %+v, %v", got, err)
	}
}

func TestClosePreservesMediaThatFailedToRestore(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "phone.wbt"), []byte("boot"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, files, _, contentID, err := inspectFirmwareDirectory(frontend.OpenRequest{Path: directory})
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		_ = file.Close()
	}
	root := t.TempDir()
	path := filepath.Join(root, contentID+".arammedia")
	if err := os.WriteFile(path, []byte("unreadable original media"), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := NewBackend(Options{MediaRoot: root, CPUBackendMode: systemmachine.CPUBackendPrecise, newMachine: func(firmwareset.Set, systemmachine.Options) (systemMachine, error) {
		return newFakeSystemMachine(), nil
	}})
	if _, err := backend.Open(context.Background(), frontend.OpenRequest{Path: directory}); err != nil {
		t.Fatal(err)
	}
	if backend.mediaWarning == "" {
		t.Fatal("restore failure did not warn")
	}
	if err := backend.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "unreadable original media" {
		t.Fatalf("unreadable original overwritten: %q, %v", data, err)
	}
}

func TestOpenRetainsOldSessionWhenMediaSaveFails(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "phone.wbt"), []byte("boot"), 0o600); err != nil {
		t.Fatal(err)
	}
	old, next := newFakeSystemMachine(), newFakeSystemMachine()
	root := filepath.Join(t.TempDir(), "media")
	if err := os.WriteFile(root, []byte("blocker"), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := NewBackend(Options{MediaRoot: root, CPUBackendMode: systemmachine.CPUBackendPrecise, newMachine: func(firmwareset.Set, systemmachine.Options) (systemMachine, error) { return next, nil }})
	backend.machine, backend.contentID = old, "synthetic"
	backend.input = frontend.InputInfo{SHA256: "synthetic"}
	if _, err := backend.Open(context.Background(), frontend.OpenRequest{Path: directory}); err == nil {
		t.Fatal("replacement succeeded after old media save failed")
	}
	if backend.machine != old || old.closed || backend.contentID != "synthetic" || !next.closed {
		t.Fatal("replacement did not preserve old session and dispose new candidate")
	}
}
