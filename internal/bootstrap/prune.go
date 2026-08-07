package bootstrap

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	runtimeTrashPrefix   = ".trash-"
	runtimeStagingPrefix = ".install-"
	stagingGracePeriod   = 24 * time.Hour
)

// PruneRuntimes deletes installed runtimes the product can no longer reach. It
// keeps the selected runtime, the runtime this process is running from, and
// keepPrevious of the most recently installed remainder. The runtime that
// installs an update is still alive while its replacement starts, so keeping
// one previous runtime leaves that process whole until the next launch.
//
// Each runtime is renamed aside before it is deleted. Windows refuses to rename
// a directory holding a running executable, so a runtime still in use is left
// intact instead of being emptied file by file, which would leave behind a
// hollow directory that Install would later mistake for a complete runtime.
func PruneRuntimes(keepPrevious int) error {
	if keepPrevious < 0 {
		keepPrevious = 0
	}
	root, err := runtimeDirectory()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect installed runtimes: %w", err)
	}
	protected := protectedRuntimes(root)
	var candidates []os.DirEntry
	for _, entry := range entries {
		name := entry.Name()
		switch {
		case strings.HasPrefix(name, runtimeTrashPrefix):
			discardRuntime(root, name)
		case strings.HasPrefix(name, runtimeStagingPrefix):
			if abandonedStaging(entry) {
				discardRuntime(root, name)
			}
		case !entry.IsDir():
			continue
		default:
			if _, keep := protected[name]; !keep {
				candidates = append(candidates, entry)
			}
		}
	}
	sortByInstalledFirst(candidates)
	for index, entry := range candidates {
		if index >= keepPrevious {
			discardRuntime(root, entry.Name())
		}
	}
	return nil
}

// protectedRuntimes names the runtimes that must survive: the one the marker
// selects and the one this process was started from. They are usually the same
// directory, and differ while an update is being installed.
func protectedRuntimes(root string) map[string]struct{} {
	protected := make(map[string]struct{}, 2)
	if marker, err := readCurrentRuntime(); err == nil {
		if selected, err := selectedExecutable(marker); err == nil {
			if name, ok := runtimeOwning(root, selected); ok {
				protected[name] = struct{}{}
			}
		}
	}
	if current, err := os.Executable(); err == nil {
		if absolute, err := filepath.Abs(current); err == nil {
			if name, ok := runtimeOwning(root, absolute); ok {
				protected[name] = struct{}{}
			}
		}
	}
	return protected
}

// runtimeOwning reports which runtime directory holds target. macOS runs the
// executable inside an application bundle, so the running path sits several
// levels below the runtime rather than directly inside it.
func runtimeOwning(root string, target string) (string, bool) {
	if !within(root, target) {
		return "", false
	}
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return "", false
	}
	name, _, _ := strings.Cut(relative, string(filepath.Separator))
	if name == "" || name == "." || name == ".." {
		return "", false
	}
	return name, true
}

// discardRuntime renames a runtime out of the way and then deletes it. A
// rename that fails means the runtime is still in use, and it is left alone. A
// deletion interrupted partway leaves a renamed directory that the next prune
// finishes off.
func discardRuntime(root string, name string) {
	target := filepath.Join(root, name)
	if !within(root, target) || samePath(root, target) {
		return
	}
	if strings.HasPrefix(name, runtimeTrashPrefix) {
		_ = os.RemoveAll(target)
		return
	}
	trash := filepath.Join(root, runtimeTrashPrefix+name)
	if _, err := os.Lstat(trash); err == nil {
		if err := os.RemoveAll(trash); err != nil {
			return
		}
	}
	if err := os.Rename(target, trash); err != nil {
		return
	}
	_ = os.RemoveAll(trash)
}

// abandonedStaging reports whether a staging directory is old enough that no
// installation can still be writing to it.
func abandonedStaging(entry os.DirEntry) bool {
	info, err := entry.Info()
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) > stagingGracePeriod
}

func sortByInstalledFirst(entries []os.DirEntry) {
	sort.Slice(entries, func(left int, right int) bool {
		leftTime := installedAt(entries[left])
		rightTime := installedAt(entries[right])
		if leftTime.Equal(rightTime) {
			return entries[left].Name() < entries[right].Name()
		}
		return leftTime.After(rightTime)
	})
}

func installedAt(entry os.DirEntry) time.Time {
	info, err := entry.Info()
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}
