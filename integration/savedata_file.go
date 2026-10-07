//go:build !js || !wasm

package integration

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mirusu400/aram-emu/internal/savefile"
)

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

func (backend *Backend) readSaveData(hash string) ([]byte, error) {
	path, err := backend.saveDataFileFor(hash)
	if err != nil {
		return nil, fmt.Errorf("resolve game save path: %w", err)
	}
	data, err := savefile.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read game save data: %w", err)
	}
	return data, nil
}

func (backend *Backend) writeSaveDataBlob(hash string, data []byte) error {
	path, err := backend.saveDataFileFor(hash)
	if err != nil {
		return fmt.Errorf("resolve game save path: %w", err)
	}
	file, err := os.CreateTemp(filepath.Dir(path), "savedata-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary game save: %w", err)
	}
	temporary := file.Name()
	committed := false
	defer func() {
		if !committed {
			_ = file.Close()
			_ = os.Remove(temporary)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write temporary game save: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync temporary game save: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temporary game save: %w", err)
	}
	if err := replaceFileCrashSafely(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("replace game save: %w", err)
	}
	committed = true
	return nil
}
