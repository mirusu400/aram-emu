package savefile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func expect(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Fatalf("read %q = %q, %v; want %q", path, got, err, want)
	}
}

func TestRecoverKeepsExistingTargetAndBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save")
	write(t, path, "current progress")
	write(t, path+".bak", "earlier progress")
	if err := Recover(path); err != nil {
		t.Fatal(err)
	}
	expect(t, path, "current progress")
	expect(t, path+".bak", "earlier progress")
}

func TestReplacePreservesExistingBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save")
	write(t, path, "current progress")
	write(t, path+".bak", "earlier progress")
	write(t, path+".tmp", "new progress")
	if err := Replace(path+".tmp", path); err != nil {
		t.Fatal(err)
	}
	expect(t, path, "new progress")
	archives, err := filepath.Glob(path + ".bak-preserved-*")
	if err != nil || len(archives) != 1 {
		t.Fatalf("preserved backups = %v, %v", archives, err)
	}
	expect(t, archives[0], "earlier progress")
}

func TestReplaceFailureRestoresPreviousTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save")
	temporary := path + ".tmp"
	write(t, path, "current progress")
	write(t, temporary, "new progress")
	installErr := errors.New("synthetic install failure")
	err := replace(temporary, path, func(from, to string) error {
		if from == temporary {
			return installErr
		}
		return os.Rename(from, to)
	})
	if !errors.Is(err, installErr) {
		t.Fatalf("replace error = %v", err)
	}
	expect(t, path, "current progress")
	expect(t, temporary, "new progress")
}

func TestFailedRollbackLeavesRecoverableBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save")
	temporary := path + ".tmp"
	write(t, path, "current progress")
	write(t, temporary, "new progress")
	installErr, rollbackErr := errors.New("synthetic install failure"), errors.New("synthetic rollback failure")
	err := replace(temporary, path, func(from, to string) error {
		if from == temporary {
			return installErr
		}
		if from == path+".bak" {
			return rollbackErr
		}
		return os.Rename(from, to)
	})
	if !errors.Is(err, installErr) || !errors.Is(err, rollbackErr) {
		t.Fatalf("replace lost error = %v", err)
	}
	expect(t, path+".bak", "current progress")
	data, err := ReadFile(path)
	if err != nil || string(data) != "current progress" {
		t.Fatalf("recovered save = %q, %v", data, err)
	}
}

func TestInvalidTemporaryLeavesTargetAndBackupIntact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save")
	write(t, path, "current progress")
	write(t, path+".bak", "earlier progress")
	if err := Replace(path+".missing", path); err == nil {
		t.Fatal("missing temporary accepted")
	}
	expect(t, path, "current progress")
	expect(t, path+".bak", "earlier progress")
}

func TestBackupDirectoryCannotReplaceExistingSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save")
	write(t, path, "current progress")
	write(t, path+".tmp", "new progress")
	if err := os.Mkdir(path+".bak", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := Replace(path+".tmp", path); err == nil {
		t.Fatal("non-file backup accepted")
	}
	expect(t, path, "current progress")
}
