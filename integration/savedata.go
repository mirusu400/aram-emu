package integration

import (
	"errors"
	"os"
	"path/filepath"

	aramcore "github.com/mirusu400/aram-core/core"
)

// saveDataMachine is the optional core capability that carries a title's
// writable storage across relaunches, the way handset flash survives an app
// exit. A machine that lacks it (no persistent storage) is simply not persisted.
type saveDataMachine interface {
	ExportSaveData() ([]byte, error)
	ImportSaveData([]byte) error
}

// currentInputHash returns the loaded title's SHA-256 identity, the key for its
// state slots and save-data file.
func (backend *Backend) currentInputHash() string {
	backend.mu.RLock()
	defer backend.mu.RUnlock()
	return backend.input.SHA256
}

// saveDataFrom reaches the save-data capability through the cheat wrapper.
func saveDataFrom(machine aramcore.Machine) (saveDataMachine, bool) {
	if machine == nil {
		return nil, false
	}
	if capability, ok := unwrapMachine(machine).(saveDataMachine); ok {
		return capability, true
	}
	capability, ok := machine.(saveDataMachine)
	return capability, ok
}

// saveDataFileFor returns the per-title save-data path, keyed by input SHA-256
// so each title keeps its own flash image alongside its state slots.
func (backend *Backend) saveDataFileFor(hash string) (string, error) {
	if hash == "" {
		return "", errors.New("loaded input has no SHA-256 identity")
	}
	backend.mu.RLock()
	root := backend.stateRoot
	backend.mu.RUnlock()
	if root == "" {
		configRoot, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(configRoot, "ARAM", "states")
	}
	directory := filepath.Join(root, hash)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(directory, "savedata.bin"), nil
}

// restoreSaveData loads a title's persisted writable storage into a freshly
// opened machine before it starts, so the guest's first read observes its
// saves. A missing file (first launch) is not an error.
func (backend *Backend) restoreSaveData(machine aramcore.Machine, hash string) {
	capability, ok := saveDataFrom(machine)
	if !ok {
		return
	}
	path, err := backend.saveDataFileFor(hash)
	if err != nil {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	_ = capability.ImportSaveData(data)
}

// persistSaveData writes a title's writable storage to its per-title file. An
// empty export writes nothing, so a title that never saved leaves no file.
func (backend *Backend) persistSaveData(machine aramcore.Machine, hash string) {
	capability, ok := saveDataFrom(machine)
	if !ok {
		return
	}
	data, err := capability.ExportSaveData()
	if err != nil || len(data) == 0 {
		return
	}
	path, err := backend.saveDataFileFor(hash)
	if err != nil {
		return
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
	}
}
