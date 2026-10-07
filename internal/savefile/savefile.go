// Package savefile replaces local saves while retaining the previous file until
// the new file is installed. It recovers the .bak left by an interrupted replace.
package savefile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func regularFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("save path %q is not a regular file", path)
	}
	return nil
}

// Recover restores a backup only when its target is absent. When both exist,
// neither is changed: readers use the target, and Replace preserves the backup.
func Recover(path string) error {
	if err := regularFile(path); !errors.Is(err, os.ErrNotExist) {
		return err
	}
	backup := path + ".bak"
	if err := regularFile(backup); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("inspect interrupted save backup: %w", err)
	}
	if err := os.Rename(backup, path); err != nil {
		return fmt.Errorf("recover interrupted save: %w", err)
	}
	return nil
}

func Open(path string) (*os.File, error) {
	if err := Recover(path); err != nil {
		return nil, err
	}
	return os.Open(path)
}

func ReadFile(path string) ([]byte, error) {
	file, err := Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(file)
}

// Replace installs an already synced and closed temporary file. Any failed
// install restores the old target; a failed rollback leaves it in .bak for the
// next read. A pre-existing .bak is archived instead of being overwritten.
func Replace(temporaryPath, targetPath string) error {
	return replace(temporaryPath, targetPath, os.Rename)
}

func replace(temporaryPath, targetPath string, rename func(string, string) error) error {
	// Validate before recovery or any rename so an invalid temporary cannot
	// disturb the target or its only surviving backup.
	if err := regularFile(temporaryPath); err != nil {
		return fmt.Errorf("inspect temporary save: %w", err)
	}
	if err := Recover(targetPath); err != nil {
		return err
	}
	backup := targetPath + ".bak"
	hadTarget := false
	if err := regularFile(targetPath); err == nil {
		if err := preserveBackup(backup); err != nil {
			return err
		}
		if err := rename(targetPath, backup); err != nil {
			return fmt.Errorf("retain previous save: %w", err)
		}
		hadTarget = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := rename(temporaryPath, targetPath); err != nil {
		installErr := fmt.Errorf("install save: %w", err)
		if hadTarget {
			if rollbackErr := rename(backup, targetPath); rollbackErr != nil {
				return errors.Join(installErr, fmt.Errorf("restore previous save (retained at %q): %w", backup, rollbackErr))
			}
		}
		return installErr
	}
	// A leftover backup is safe and will be preserved by a later replacement.
	if hadTarget {
		_ = os.Remove(backup)
	}
	return nil
}

func preserveBackup(path string) error {
	if err := regularFile(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	backup, err := os.Open(path)
	if err != nil {
		return err
	}
	defer backup.Close()
	archive, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+"-preserved-*")
	if err != nil {
		return fmt.Errorf("preserve existing save backup: %w", err)
	}
	complete := false
	defer func() {
		_ = archive.Close()
		if !complete {
			_ = os.Remove(archive.Name())
		}
	}()
	if _, err := io.Copy(archive, backup); err != nil {
		return err
	}
	if err := archive.Sync(); err != nil {
		return err
	}
	if err := archive.Close(); err != nil {
		return err
	}
	if err := backup.Close(); err != nil {
		return err
	}
	complete = true
	return os.Remove(path)
}
