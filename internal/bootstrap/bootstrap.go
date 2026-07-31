package bootstrap

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	maxArchiveEntries = 4096
	maxExtractedBytes = int64(2 << 30)
	maxMarkerBytes    = 64 << 10
)

type currentRuntime struct {
	Executable         string    `json:"executable"`
	ArchiveSHA256      string    `json:"archive_sha256"`
	LauncherExecutable string    `json:"launcher_executable,omitempty"`
	LauncherSHA256     string    `json:"launcher_sha256,omitempty"`
	InstalledAt        time.Time `json:"installed_at"`
}

// ForwardToInstalled starts the currently installed runtime when the user
// launches an older bootstrap copy. The installed runtime recognizes itself
// by its absolute path and continues in-process instead of forwarding again.
func ForwardToInstalled(args []string) (bool, error) {
	marker, err := readCurrentRuntime()
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	installed, err := selectedExecutable(marker)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	current, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("locate bootstrap executable: %w", err)
	}
	current, err = filepath.Abs(current)
	if err != nil {
		return false, err
	}
	currentDigest, err := fileSHA256(current)
	if err != nil {
		return false, err
	}
	if !shouldForward(marker, current, currentDigest, installed) {
		return false, nil
	}
	if err := Launch(installed, args); err != nil {
		return false, err
	}
	return true, nil
}

// Install extracts a verified product archive into a content-addressed runtime
// directory and atomically selects it for subsequent launches.
func Install(archivePath string) (string, error) {
	archivePath, err := filepath.Abs(archivePath)
	if err != nil {
		return "", fmt.Errorf("resolve product archive: %w", err)
	}
	info, err := os.Stat(archivePath)
	if err != nil {
		return "", fmt.Errorf("inspect product archive: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("product archive is not a regular file")
	}
	digest, err := fileSHA256(archivePath)
	if err != nil {
		return "", err
	}
	root, err := runtimeDirectory()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("create runtime directory: %w", err)
	}
	directory := filepath.Join(root, digest[:16])
	executable := filepath.Join(directory, productExecutableName())
	if regularFile(executable) {
		if err := writeCurrentRuntime(executable, digest); err != nil {
			return "", err
		}
		return executable, nil
	}

	temporary, err := os.MkdirTemp(root, ".install-*")
	if err != nil {
		return "", fmt.Errorf("create installation staging directory: %w", err)
	}
	keepTemporary := false
	defer func() {
		if !keepTemporary {
			_ = os.RemoveAll(temporary)
		}
	}()
	if err := extractArchive(archivePath, temporary); err != nil {
		return "", err
	}
	stagedExecutable := filepath.Join(temporary, productExecutableName())
	if !regularFile(stagedExecutable) {
		return "", fmt.Errorf(
			"product archive does not contain %s",
			productExecutableName(),
		)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(stagedExecutable, 0o755); err != nil {
			return "", fmt.Errorf("make product executable runnable: %w", err)
		}
	}
	if _, err := os.Stat(directory); err == nil {
		if !within(root, directory) {
			return "", errors.New("refusing to replace a runtime outside the ARAM directory")
		}
		if err := os.RemoveAll(directory); err != nil {
			return "", fmt.Errorf("replace incomplete runtime: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect runtime target: %w", err)
	}
	if err := os.Rename(temporary, directory); err != nil {
		return "", fmt.Errorf("activate installed runtime: %w", err)
	}
	keepTemporary = true
	if err := writeCurrentRuntime(executable, digest); err != nil {
		return "", err
	}
	return executable, nil
}

func CurrentExecutable() (string, error) {
	marker, err := readCurrentRuntime()
	if err != nil {
		return "", err
	}
	return selectedExecutable(marker)
}

func readCurrentRuntime() (currentRuntime, error) {
	var marker currentRuntime
	markerPath, err := currentRuntimePath()
	if err != nil {
		return marker, err
	}
	file, err := os.Open(markerPath)
	if err != nil {
		return marker, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxMarkerBytes+1))
	if err != nil {
		return marker, fmt.Errorf("read current runtime marker: %w", err)
	}
	if len(data) > maxMarkerBytes {
		return marker, errors.New("current runtime marker is too large")
	}
	if err := json.Unmarshal(data, &marker); err != nil {
		return marker, fmt.Errorf("decode current runtime marker: %w", err)
	}
	return marker, nil
}

func selectedExecutable(marker currentRuntime) (string, error) {
	root, err := runtimeDirectory()
	if err != nil {
		return "", err
	}
	executable, err := filepath.Abs(marker.Executable)
	if err != nil {
		return "", fmt.Errorf("resolve current runtime: %w", err)
	}
	if !within(root, executable) {
		return "", errors.New("current runtime points outside the ARAM runtime directory")
	}
	if !regularFile(executable) {
		return "", os.ErrNotExist
	}
	return executable, nil
}

func shouldForward(
	marker currentRuntime,
	current string,
	currentDigest string,
	installed string,
) bool {
	if samePath(current, installed) {
		return false
	}
	if marker.LauncherExecutable == "" || marker.LauncherSHA256 == "" {
		return false
	}
	return samePath(current, marker.LauncherExecutable) &&
		strings.EqualFold(currentDigest, marker.LauncherSHA256)
}

func Launch(executable string, args []string) error {
	command := exec.Command(executable, args...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		return fmt.Errorf("launch installed ARAM runtime: %w", err)
	}
	return nil
}

func writeCurrentRuntime(executable string, digest string) error {
	markerPath, err := currentRuntimePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(markerPath), 0o700); err != nil {
		return fmt.Errorf("create ARAM configuration directory: %w", err)
	}
	var existing currentRuntime
	if marker, readErr := readCurrentRuntime(); readErr == nil {
		existing = marker
	}
	launcher, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate launcher executable: %w", err)
	}
	launcher, err = filepath.Abs(launcher)
	if err != nil {
		return fmt.Errorf("resolve launcher executable: %w", err)
	}
	launcherDigest, err := fileSHA256(launcher)
	if err != nil {
		return fmt.Errorf("hash launcher executable: %w", err)
	}
	if existing.LauncherExecutable != "" &&
		existing.LauncherSHA256 != "" &&
		!samePath(launcher, existing.LauncherExecutable) {
		launcher = existing.LauncherExecutable
		launcherDigest = existing.LauncherSHA256
	}
	data, err := json.MarshalIndent(currentRuntime{
		Executable:         executable,
		ArchiveSHA256:      digest,
		LauncherExecutable: launcher,
		LauncherSHA256:     launcherDigest,
		InstalledAt:        time.Now().UTC(),
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode current runtime marker: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(markerPath), ".current-*.json")
	if err != nil {
		return fmt.Errorf("create current runtime marker: %w", err)
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write current runtime marker: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("flush current runtime marker: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close current runtime marker: %w", err)
	}
	_ = os.Remove(markerPath)
	if err := os.Rename(temporaryPath, markerPath); err != nil {
		return fmt.Errorf("activate current runtime marker: %w", err)
	}
	committed = true
	return nil
}

func extractArchive(archivePath string, destination string) error {
	lower := strings.ToLower(archivePath)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return extractZIP(archivePath, destination)
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return extractTarGZ(archivePath, destination)
	default:
		return errors.New("product update must be a ZIP or tar.gz archive")
	}
}

func extractZIP(archivePath string, destination string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open product ZIP: %w", err)
	}
	defer reader.Close()
	if len(reader.File) > maxArchiveEntries {
		return errors.New("product ZIP contains too many entries")
	}
	var written int64
	seen := make(map[string]struct{}, len(reader.File))
	for _, entry := range reader.File {
		target, clean, err := archiveTarget(destination, entry.Name)
		if err != nil {
			return err
		}
		if clean == "." {
			continue
		}
		if _, exists := seen[clean]; exists {
			return fmt.Errorf("product ZIP contains duplicate entry %q", clean)
		}
		seen[clean] = struct{}{}
		mode := entry.Mode()
		if mode&os.ModeSymlink != 0 || (!entry.FileInfo().IsDir() && !mode.IsRegular()) {
			return fmt.Errorf("product ZIP contains unsupported entry %q", clean)
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("create product directory %q: %w", clean, err)
			}
			continue
		}
		source, err := entry.Open()
		if err != nil {
			return fmt.Errorf("open product entry %q: %w", clean, err)
		}
		if err := writeArchiveFile(source, target, mode.Perm(), &written); err != nil {
			_ = source.Close()
			return fmt.Errorf("extract product entry %q: %w", clean, err)
		}
		if err := source.Close(); err != nil {
			return fmt.Errorf("close product entry %q: %w", clean, err)
		}
	}
	return nil
}

func extractTarGZ(archivePath string, destination string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open product tar.gz: %w", err)
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("open product gzip stream: %w", err)
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	var written int64
	entries := 0
	seen := make(map[string]struct{})
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read product tar: %w", err)
		}
		entries++
		if entries > maxArchiveEntries {
			return errors.New("product tar contains too many entries")
		}
		target, clean, err := archiveTarget(destination, header.Name)
		if err != nil {
			return err
		}
		if clean == "." {
			continue
		}
		if _, exists := seen[clean]; exists {
			return fmt.Errorf("product tar contains duplicate entry %q", clean)
		}
		seen[clean] = struct{}{}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("create product directory %q: %w", clean, err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := writeArchiveFile(
				reader,
				target,
				os.FileMode(header.Mode).Perm(),
				&written,
			); err != nil {
				return fmt.Errorf("extract product entry %q: %w", clean, err)
			}
		default:
			return fmt.Errorf("product tar contains unsupported entry %q", clean)
		}
	}
	return nil
}

func writeArchiveFile(
	source io.Reader,
	target string,
	mode os.FileMode,
	written *int64,
) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if mode == 0 {
		mode = 0o644
	}
	file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	remaining := maxExtractedBytes - *written
	count, copyErr := io.Copy(file, io.LimitReader(source, remaining+1))
	*written += count
	closeErr := file.Close()
	if count > remaining {
		return errors.New("product archive exceeds the 2 GiB extraction limit")
	}
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func archiveTarget(destination string, name string) (string, string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	clean := path.Clean(name)
	platformPath := filepath.FromSlash(clean)
	if path.IsAbs(clean) || filepath.VolumeName(platformPath) != "" ||
		clean == ".." || strings.HasPrefix(clean, "../") {
		return "", "", fmt.Errorf("product archive entry %q escapes its destination", name)
	}
	target := filepath.Join(destination, platformPath)
	if !within(destination, target) {
		return "", "", fmt.Errorf("product archive entry %q escapes its destination", name)
	}
	return target, clean, nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open product archive: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("hash product archive: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func currentRuntimePath() (string, error) {
	root, err := configDirectory()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "runtime", "current.json"), nil
}

func runtimeDirectory() (string, error) {
	root, err := configDirectory()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "runtime", "versions"), nil
}

func configDirectory() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate ARAM configuration directory: %w", err)
	}
	return filepath.Join(root, "ARAM"), nil
}

func productExecutableName() string {
	if runtime.GOOS == "windows" {
		return "aram.exe"
	}
	return "aram"
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func within(root string, target string) bool {
	root, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func samePath(left string, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}
