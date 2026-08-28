package integration

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"strings"
)

// A save backup is a self-describing container around a title's writable
// storage. Unlike the bare savedata.bin the backend keeps in the local state
// directory, it records which title the bytes belong to and a checksum, so a
// user can copy it off the machine, keep it anywhere, and restore it later -
// even onto another install - with the wrong-title and corruption cases caught
// rather than silently loading garbage into a guest.
//
// Layout (little-endian):
//
//	magic    "ARAMSAVE"   8 bytes
//	version  uint8        1 byte
//	reserved uint8        1 byte  (zero; room for future flags)
//	identity 32 bytes             raw SHA-256 of the input the save belongs to
//	length   uint32       4 bytes payload length
//	crc32    uint32       4 bytes IEEE checksum of the payload
//	payload  length bytes         the title's writable storage
const (
	saveBackupMagic       = "ARAMSAVE"
	saveBackupVersion     = 1
	saveBackupHashBytes   = 32
	saveBackupHeaderBytes = len(saveBackupMagic) + 1 + 1 + saveBackupHashBytes + 4 + 4
)

// encodeSaveBackup wraps a title's writable storage in a backup container keyed
// to the input's SHA-256 hex identity.
func encodeSaveBackup(hashHex string, payload []byte) ([]byte, error) {
	identity, err := hex.DecodeString(hashHex)
	if err != nil {
		return nil, fmt.Errorf("save backup: title identity is not hex: %w", err)
	}
	if len(identity) != saveBackupHashBytes {
		return nil, fmt.Errorf(
			"save backup: title identity is %d bytes, want %d",
			len(identity), saveBackupHashBytes,
		)
	}
	out := make([]byte, 0, saveBackupHeaderBytes+len(payload))
	out = append(out, saveBackupMagic...)
	out = append(out, saveBackupVersion, 0)
	out = append(out, identity...)
	var scratch [4]byte
	binary.LittleEndian.PutUint32(scratch[:], uint32(len(payload)))
	out = append(out, scratch[:]...)
	binary.LittleEndian.PutUint32(scratch[:], crc32.ChecksumIEEE(payload))
	out = append(out, scratch[:]...)
	out = append(out, payload...)
	return out, nil
}

// decodeSaveBackup validates a backup container and returns the title identity
// it belongs to (SHA-256 hex) together with a fresh copy of the payload.
func decodeSaveBackup(data []byte) (string, []byte, error) {
	if len(data) < saveBackupHeaderBytes {
		return "", nil, errors.New("save backup: file is too small or truncated")
	}
	if string(data[:len(saveBackupMagic)]) != saveBackupMagic {
		return "", nil, errors.New("save backup: not an ARAM save backup file")
	}
	offset := len(saveBackupMagic)
	if version := data[offset]; version != saveBackupVersion {
		return "", nil, fmt.Errorf("save backup: unsupported version %d", version)
	}
	offset += 2 // version + reserved
	identity := data[offset : offset+saveBackupHashBytes]
	offset += saveBackupHashBytes
	length := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4
	wantCRC := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4
	payload := data[offset:]
	if uint32(len(payload)) != length {
		return "", nil, fmt.Errorf(
			"save backup: payload is %d bytes, header declares %d",
			len(payload), length,
		)
	}
	if crc32.ChecksumIEEE(payload) != wantCRC {
		return "", nil, errors.New("save backup: file is corrupt (checksum mismatch)")
	}
	return hex.EncodeToString(identity), append([]byte(nil), payload...), nil
}

// shortSaveHash abbreviates a SHA-256 hex identity for a human-readable error.
func shortSaveHash(hash string) string {
	if len(hash) <= 8 {
		return hash
	}
	return hash[:8]
}

// ExportSaveData returns the loaded title's writable storage wrapped in a
// portable backup container. It fails when no title is loaded, the title keeps
// no persistent storage, or the title has not written a save yet - there is
// nothing to back up in those cases rather than an empty file to restore.
func (backend *Backend) ExportSaveData() ([]byte, error) {
	backend.operationMu.Lock()
	defer backend.operationMu.Unlock()

	machine := backend.currentMachine()
	if machine == nil {
		return nil, errors.New("no title is loaded to back up")
	}
	capability, ok := saveDataFrom(machine)
	if !ok {
		return nil, errors.New("the loaded title has no writable storage to back up")
	}
	payload, err := capability.ExportSaveData()
	if err != nil {
		return nil, err
	}
	if len(payload) == 0 {
		return nil, errors.New("the loaded title has not written any save data yet")
	}
	return encodeSaveBackup(backend.currentInputHash(), payload)
}

// ImportSaveData restores a backup container into the loaded title. It refuses a
// backup that belongs to a different title, applies the storage to the running
// machine, and writes it through to the title's local save file so the restore
// survives the next launch.
func (backend *Backend) ImportSaveData(data []byte) error {
	backend.operationMu.Lock()
	defer backend.operationMu.Unlock()

	machine := backend.currentMachine()
	if machine == nil {
		return errors.New("no title is loaded to restore into")
	}
	capability, ok := saveDataFrom(machine)
	if !ok {
		return errors.New("the loaded title has no writable storage to restore into")
	}
	identity, payload, err := decodeSaveBackup(data)
	if err != nil {
		return err
	}
	current := backend.currentInputHash()
	if !strings.EqualFold(identity, current) {
		return fmt.Errorf(
			"this save backup belongs to a different title (%s…), not the loaded one",
			shortSaveHash(identity),
		)
	}
	if err := capability.ImportSaveData(payload); err != nil {
		return err
	}
	backend.persistSaveData(machine, current)
	return nil
}
