package integration

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	aramcore "github.com/mirusu400/aram-core/core"
	"github.com/mirusu400/aram-frontend/frontend"
)

// saveStubMachine is a core stand-in that implements only the writable-storage
// capability, enough to drive save backup export and import.
type saveStubMachine struct {
	aramcore.Machine
	data []byte
}

func (m *saveStubMachine) ExportSaveData() ([]byte, error) {
	return append([]byte(nil), m.data...), nil
}

func (m *saveStubMachine) ImportSaveData(data []byte) error {
	m.data = append([]byte(nil), data...)
	return nil
}

const (
	saveHashA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	saveHashB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func TestSaveBackupContainerRoundTrip(t *testing.T) {
	payload := []byte("gopt.sav contents")
	blob, err := encodeSaveBackup(saveHashA, payload)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !bytes.HasPrefix(blob, []byte(saveBackupMagic)) {
		t.Fatal("container is missing its magic")
	}
	hash, decoded, err := decodeSaveBackup(blob)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if hash != saveHashA {
		t.Fatalf("decoded identity = %q, want %q", hash, saveHashA)
	}
	if !bytes.Equal(decoded, payload) {
		t.Fatalf("decoded payload = %q, want %q", decoded, payload)
	}
}

func TestSaveBackupDecodeRejectsBadContainers(t *testing.T) {
	good, err := encodeSaveBackup(saveHashA, []byte("save"))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	if _, _, err := decodeSaveBackup(good[:saveBackupHeaderBytes-1]); err == nil {
		t.Fatal("a truncated container was accepted")
	}

	badMagic := append([]byte(nil), good...)
	badMagic[0] = 'X'
	if _, _, err := decodeSaveBackup(badMagic); err == nil {
		t.Fatal("a container with a wrong magic was accepted")
	}

	corrupt := append([]byte(nil), good...)
	corrupt[len(corrupt)-1] ^= 0xff
	if _, _, err := decodeSaveBackup(corrupt); err == nil {
		t.Fatal("a corrupt payload passed the checksum")
	}
}

func newSaveBackend(t *testing.T, hash string, machine *saveStubMachine) *Backend {
	t.Helper()
	backend := &Backend{stateRoot: t.TempDir()}
	backend.machine = machine
	backend.input = frontend.InputInfo{SHA256: hash}
	return backend
}

func TestBackendSaveExportImportRoundTrip(t *testing.T) {
	source := newSaveBackend(t, saveHashA, &saveStubMachine{data: []byte("hero level 42")})
	blob, err := source.ExportSaveData()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	target := newSaveBackend(t, saveHashA, &saveStubMachine{})
	if err := target.ImportSaveData(blob); err != nil {
		t.Fatalf("import: %v", err)
	}
	if got := string(target.machine.(*saveStubMachine).data); got != "hero level 42" {
		t.Fatalf("restored save = %q, want %q", got, "hero level 42")
	}

	// Import must write the restore through to the local save file so it
	// survives the next launch, not just the running machine.
	savePath := filepath.Join(target.stateRoot, saveHashA, "savedata.bin")
	written, err := os.ReadFile(savePath)
	if err != nil {
		t.Fatalf("save file not written on import: %v", err)
	}
	if string(written) != "hero level 42" {
		t.Fatalf("save file = %q, want %q", written, "hero level 42")
	}
}

func TestBackendSaveImportRejectsWrongTitle(t *testing.T) {
	source := newSaveBackend(t, saveHashA, &saveStubMachine{data: []byte("save-A")})
	blob, err := source.ExportSaveData()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	target := newSaveBackend(t, saveHashB, &saveStubMachine{data: []byte("save-B")})
	err = target.ImportSaveData(blob)
	if err == nil {
		t.Fatal("a backup from a different title was accepted")
	}
	if !strings.Contains(err.Error(), "different title") {
		t.Fatalf("error = %v, want it to name the title mismatch", err)
	}
	if got := string(target.machine.(*saveStubMachine).data); got != "save-B" {
		t.Fatalf("rejected import still mutated the save: %q", got)
	}
}

func TestBackendSaveExportRefusesEmptyAndAbsent(t *testing.T) {
	empty := newSaveBackend(t, saveHashA, &saveStubMachine{})
	if _, err := empty.ExportSaveData(); err == nil {
		t.Fatal("exported a backup for a title that never saved")
	}

	none := &Backend{stateRoot: t.TempDir()}
	none.input = frontend.InputInfo{SHA256: saveHashA}
	if _, err := none.ExportSaveData(); err == nil {
		t.Fatal("exported a backup with no title loaded")
	}
	if err := none.ImportSaveData([]byte("x")); err == nil {
		t.Fatal("imported a backup with no title loaded")
	}
}
