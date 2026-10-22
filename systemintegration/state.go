package systemintegration

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mirusu400/aram-core/cpu"
	"github.com/mirusu400/aram-core/systemmachine"
	"github.com/mirusu400/aram-emu/internal/savefile"
)

const (
	systemStateSchemaVersion = uint32(1)
	maxSystemStateHeaderSize = uint32(64 * 1024)
	maxSystemStateBytes      = uint64(1 << 30)
)

var systemStateMagic = []byte("ARAMSYSTEMSTATE\x00")

type systemStateHeader struct {
	Schema          uint32       `json:"schema"`
	InputSHA256     string       `json:"input_sha256"`
	ProfileID       string       `json:"profile_id"`
	Backend         string       `json:"backend"`
	SnapshotSchema  string       `json:"snapshot_schema"`
	FirmwareBuildID string       `json:"firmware_build_id"`
	BoardID         string       `json:"board_id"`
	PlatformID      string       `json:"platform_id"`
	CPUIdentity     cpu.Identity `json:"cpu_identity"`
	Instructions    uint64       `json:"instructions"`
	SectionLengths  [5]uint64    `json:"section_lengths"`
}

type rewindEntry struct {
	snapshot systemmachine.Snapshot
	bytes    uint64
}

// ConfigureStateRoot isolates whole-phone save-state slots for tests and tools.
// It must be configured before a firmware machine is opened.
func (backend *Backend) ConfigureStateRoot(root string) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return errors.New("state root is empty")
	}
	backend.operationMu.Lock()
	defer backend.operationMu.Unlock()
	backend.mu.RLock()
	open := backend.machine != nil
	backend.mu.RUnlock()
	if open {
		return errors.New("state root cannot change while a machine is open")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve state root: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return fmt.Errorf("create state root: %w", err)
	}
	backend.mu.Lock()
	backend.stateRoot = absolute
	backend.mu.Unlock()
	return nil
}

func (backend *Backend) saveState(machine systemMachine, slot int) error {
	path, err := backend.statePath(slot)
	if err != nil {
		return err
	}
	snapshot, err := machine.SaveSnapshot()
	if err != nil {
		return fmt.Errorf("capture whole-phone snapshot: %w", err)
	}
	backend.mu.RLock()
	input := backend.input
	backend.mu.RUnlock()
	if snapshot.FirmwareBuildID != input.ProfileID {
		return errors.New("whole-phone snapshot profile does not match the loaded firmware")
	}
	header := headerForSystemSnapshot(input.SHA256, input.ProfileID, backend.BackendName(), snapshot)
	if err := writeSystemStateFile(path, header, snapshot); err != nil {
		return fmt.Errorf("write whole-phone state slot %d: %w", slot, err)
	}
	return nil
}

func (backend *Backend) loadState(machine systemMachine, slot int) error {
	path, err := backend.statePath(slot)
	if err != nil {
		return err
	}
	header, snapshot, err := readSystemStateFile(path)
	if err != nil {
		return fmt.Errorf("read whole-phone state slot %d: %w", slot, err)
	}
	if err := backend.validateSystemStateHeader(header); err != nil {
		return err
	}
	if err := machine.LoadSnapshot(snapshot); err != nil {
		return fmt.Errorf("restore whole-phone snapshot: %w", err)
	}
	return nil
}

func (backend *Backend) statePath(slot int) (string, error) {
	if slot < 0 || slot > 9 {
		return "", fmt.Errorf("state slot %d is outside 0-9", slot)
	}
	backend.mu.RLock()
	root := backend.stateRoot
	hash := backend.input.SHA256
	backend.mu.RUnlock()
	if root == "" {
		configRoot, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(configRoot, "ARAM", "states")
	} else {
		absolute, err := filepath.Abs(root)
		if err != nil {
			return "", fmt.Errorf("resolve state root: %w", err)
		}
		root = absolute
	}
	if hash == "" {
		return "", errors.New("loaded firmware has no SHA-256 identity")
	}
	directory := filepath.Join(root, hash)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(directory, fmt.Sprintf("slot-%d.aramstate", slot)), nil
}

func (backend *Backend) validateSystemStateHeader(header systemStateHeader) error {
	backend.mu.RLock()
	input := backend.input
	backend.mu.RUnlock()
	switch {
	case header.Schema != systemStateSchemaVersion:
		return fmt.Errorf("system state schema %d is unsupported", header.Schema)
	case header.InputSHA256 != input.SHA256:
		return errors.New("system state belongs to a different firmware hash")
	case header.ProfileID != input.ProfileID:
		return errors.New("system state belongs to a different firmware profile")
	case header.Backend != backend.BackendName():
		return errors.New("system state belongs to a different backend")
	case header.SnapshotSchema != systemmachine.SnapshotSchema:
		return fmt.Errorf("core snapshot schema %q is unsupported", header.SnapshotSchema)
	default:
		return nil
	}
}

func headerForSystemSnapshot(inputHash, profileID, backendName string, snapshot systemmachine.Snapshot) systemStateHeader {
	return systemStateHeader{
		Schema:          systemStateSchemaVersion,
		InputSHA256:     inputHash,
		ProfileID:       profileID,
		Backend:         backendName,
		SnapshotSchema:  snapshot.Schema,
		FirmwareBuildID: snapshot.FirmwareBuildID,
		BoardID:         snapshot.BoardID,
		PlatformID:      snapshot.PlatformID,
		CPUIdentity:     snapshot.CPUIdentity,
		Instructions:    snapshot.Instructions,
		SectionLengths: [5]uint64{
			uint64(len(snapshot.CPU)),
			uint64(len(snapshot.Bus)),
			uint64(len(snapshot.Flash)),
			uint64(len(snapshot.SecondaryFlash)),
			uint64(len(snapshot.OneNANDSpare)),
		},
	}
}

func writeSystemStateFile(path string, header systemStateHeader, snapshot systemmachine.Snapshot) error {
	expectedHeader := headerForSystemSnapshot(
		header.InputSHA256,
		header.ProfileID,
		header.Backend,
		snapshot,
	)
	if header != expectedHeader {
		return errors.New("system state header does not match the snapshot")
	}
	if err := validateSystemStateStructure(header); err != nil {
		return err
	}
	headerData, err := json.Marshal(header)
	if err != nil {
		return err
	}
	if len(headerData) == 0 || len(headerData) > int(maxSystemStateHeaderSize) {
		return errors.New("system state header exceeds the safety limit")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".system-state-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		if !committed {
			_ = temporary.Close()
			_ = os.Remove(temporaryPath)
		}
	}()
	if _, err := temporary.Write(systemStateMagic); err != nil {
		return err
	}
	hash := sha256.New()
	output := io.MultiWriter(temporary, hash)
	if err := binary.Write(output, binary.LittleEndian, uint32(len(headerData))); err != nil {
		return err
	}
	if _, err := output.Write(headerData); err != nil {
		return err
	}
	for _, section := range systemSnapshotSections(snapshot) {
		if _, err := output.Write(section); err != nil {
			return err
		}
	}
	if _, err := temporary.Write(hash.Sum(nil)); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := savefile.Replace(temporaryPath, path); err != nil {
		return err
	}
	committed = true
	return nil
}

func readSystemStateFile(path string) (systemStateHeader, systemmachine.Snapshot, error) {
	file, err := savefile.Open(path)
	if err != nil {
		return systemStateHeader{}, systemmachine.Snapshot{}, err
	}
	defer file.Close()
	magic := make([]byte, len(systemStateMagic))
	if _, err := io.ReadFull(file, magic); err != nil {
		return systemStateHeader{}, systemmachine.Snapshot{}, fmt.Errorf("read system state magic: %w", err)
	}
	if !bytes.Equal(magic, systemStateMagic) {
		return systemStateHeader{}, systemmachine.Snapshot{}, errors.New("system state file has invalid magic")
	}
	hash := sha256.New()
	input := io.TeeReader(file, hash)
	var headerSize uint32
	if err := binary.Read(input, binary.LittleEndian, &headerSize); err != nil {
		return systemStateHeader{}, systemmachine.Snapshot{}, fmt.Errorf("read system state header size: %w", err)
	}
	if headerSize == 0 || headerSize > maxSystemStateHeaderSize {
		return systemStateHeader{}, systemmachine.Snapshot{}, errors.New("system state header size is invalid")
	}
	headerData := make([]byte, int(headerSize))
	if _, err := io.ReadFull(input, headerData); err != nil {
		return systemStateHeader{}, systemmachine.Snapshot{}, fmt.Errorf("read system state header: %w", err)
	}
	var header systemStateHeader
	if err := json.Unmarshal(headerData, &header); err != nil {
		return systemStateHeader{}, systemmachine.Snapshot{}, fmt.Errorf("decode system state header: %w", err)
	}
	if err := validateSystemStateStructure(header); err != nil {
		return systemStateHeader{}, systemmachine.Snapshot{}, err
	}
	sections := make([][]byte, len(header.SectionLengths))
	for index, length := range header.SectionLengths {
		if length == 0 {
			continue
		}
		sections[index] = make([]byte, int(length))
		if _, err := io.ReadFull(input, sections[index]); err != nil {
			return systemStateHeader{}, systemmachine.Snapshot{}, fmt.Errorf("read system state section %d: %w", index, err)
		}
	}
	wantDigest := make([]byte, sha256.Size)
	if _, err := io.ReadFull(file, wantDigest); err != nil {
		return systemStateHeader{}, systemmachine.Snapshot{}, fmt.Errorf("read system state checksum: %w", err)
	}
	var trailing [1]byte
	if count, trailingErr := file.Read(trailing[:]); count != 0 || trailingErr == nil {
		return systemStateHeader{}, systemmachine.Snapshot{}, errors.New("system state file has trailing data")
	} else if !errors.Is(trailingErr, io.EOF) {
		return systemStateHeader{}, systemmachine.Snapshot{}, trailingErr
	}
	if !equalBytes(hash.Sum(nil), wantDigest) {
		return systemStateHeader{}, systemmachine.Snapshot{}, errors.New("system state checksum does not match")
	}
	return header, systemmachine.Snapshot{
		Schema:          header.SnapshotSchema,
		FirmwareBuildID: header.FirmwareBuildID,
		BoardID:         header.BoardID,
		PlatformID:      header.PlatformID,
		CPUIdentity:     header.CPUIdentity,
		CPU:             sections[0],
		Bus:             sections[1],
		Flash:           sections[2],
		SecondaryFlash:  sections[3],
		OneNANDSpare:    sections[4],
		Instructions:    header.Instructions,
	}, nil
}

func validateSystemStateStructure(header systemStateHeader) error {
	if header.Schema != systemStateSchemaVersion {
		return fmt.Errorf("system state schema %d is unsupported", header.Schema)
	}
	if header.InputSHA256 == "" || header.ProfileID == "" || header.Backend == "" ||
		header.SnapshotSchema == "" || header.FirmwareBuildID == "" ||
		header.BoardID == "" || header.PlatformID == "" {
		return errors.New("system state header is incomplete")
	}
	if err := header.CPUIdentity.Validate(); err != nil {
		return fmt.Errorf("system state CPU identity: %w", err)
	}
	if header.SectionLengths[0] == 0 || header.SectionLengths[1] == 0 || header.SectionLengths[2] == 0 {
		return errors.New("system state is missing a required snapshot section")
	}
	maxInt := uint64(^uint(0) >> 1)
	var total uint64
	for _, length := range header.SectionLengths {
		if length > maxInt || length > maxSystemStateBytes || total > maxSystemStateBytes-length {
			return errors.New("system state sections exceed the safety limit")
		}
		total += length
	}
	return nil
}

func systemSnapshotSections(snapshot systemmachine.Snapshot) [][]byte {
	return [][]byte{
		snapshot.CPU,
		snapshot.Bus,
		snapshot.Flash,
		snapshot.SecondaryFlash,
		snapshot.OneNANDSpare,
	}
}

func systemSnapshotBytes(snapshot systemmachine.Snapshot) uint64 {
	var total uint64
	for _, section := range systemSnapshotSections(snapshot) {
		total += uint64(len(section))
	}
	return total
}

// captureRewindCheckpoint is best-effort: failure to retain an auxiliary
// checkpoint must not interrupt the phone. Capability reports the reason.
func (backend *Backend) captureRewindCheckpoint(machine systemMachine) {
	backend.mu.RLock()
	shouldCapture := backend.rewindUnavailable == "" &&
		(len(backend.rewindHistory) == 0 ||
			backend.rewindFrames >= backend.options.RewindIntervalFrames ||
			backend.rewindBranchAnchor)
	backend.mu.RUnlock()
	if !shouldCapture {
		return
	}
	snapshot, err := machine.SaveSnapshot()
	if err != nil {
		backend.mu.Lock()
		backend.rewindUnavailable = "Whole-phone rewind checkpoint failed: " + err.Error()
		backend.mu.Unlock()
		return
	}
	size := systemSnapshotBytes(snapshot)
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if size == 0 || size > backend.options.RewindMemoryLimitBytes {
		backend.resetRewindHistoryLocked()
		backend.rewindUnavailable = fmt.Sprintf(
			"Whole-phone snapshot needs %d bytes, above the %d-byte rewind limit",
			size,
			backend.options.RewindMemoryLimitBytes,
		)
		return
	}
	backend.rewindHistory = append(backend.rewindHistory, rewindEntry{snapshot: snapshot, bytes: size})
	backend.rewindBytes += size
	for len(backend.rewindHistory) > backend.options.RewindCapacity ||
		backend.rewindBytes > backend.options.RewindMemoryLimitBytes {
		backend.rewindBytes -= backend.rewindHistory[0].bytes
		backend.rewindHistory[0] = rewindEntry{}
		backend.rewindHistory = backend.rewindHistory[1:]
	}
	backend.rewindFrames = 0
	backend.rewindBranchAnchor = false
}

func (backend *Backend) noteRewindFrame() {
	backend.mu.Lock()
	if backend.rewindFrames != ^uint64(0) {
		backend.rewindFrames++
	}
	backend.mu.Unlock()
}

func (backend *Backend) rewind(machine systemMachine) error {
	backend.mu.RLock()
	if len(backend.rewindHistory) == 0 {
		reason := backend.rewindUnavailable
		backend.mu.RUnlock()
		if reason == "" {
			reason = "whole-phone rewind history is empty"
		}
		return errors.New(reason)
	}
	entry := backend.rewindHistory[len(backend.rewindHistory)-1]
	backend.mu.RUnlock()
	if err := machine.LoadSnapshot(entry.snapshot); err != nil {
		return fmt.Errorf("restore whole-phone rewind checkpoint: %w", err)
	}
	backend.mu.Lock()
	last := len(backend.rewindHistory) - 1
	backend.rewindBytes -= backend.rewindHistory[last].bytes
	backend.rewindHistory[last] = rewindEntry{}
	backend.rewindHistory = backend.rewindHistory[:last]
	backend.rewindFrames = 0
	backend.rewindBranchAnchor = true
	backend.rewindUnavailable = ""
	backend.mu.Unlock()
	return nil
}

func (backend *Backend) resetRewindHistory() {
	backend.mu.Lock()
	backend.resetRewindHistoryLocked()
	backend.mu.Unlock()
}

func (backend *Backend) resetRewindHistoryLocked() {
	for index := range backend.rewindHistory {
		backend.rewindHistory[index] = rewindEntry{}
	}
	backend.rewindHistory = nil
	backend.rewindBytes = 0
	backend.rewindFrames = 0
	backend.rewindBranchAnchor = false
	backend.rewindUnavailable = ""
}
