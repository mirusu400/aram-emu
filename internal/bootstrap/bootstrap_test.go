package bootstrap

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func isolateConfig(t *testing.T) {
	t.Helper()
	temporary := t.TempDir()
	t.Setenv("APPDATA", temporary)
	t.Setenv("XDG_CONFIG_HOME", temporary)
	t.Setenv("HOME", temporary)
}

func TestInstallSelectsContentAddressedRuntime(t *testing.T) {
	for _, format := range []string{"zip", "tar.gz"} {
		t.Run(format, func(t *testing.T) {
			isolateConfig(t)
			archivePath := filepath.Join(t.TempDir(), "aram."+format)
			createProductArchive(
				t,
				archivePath,
				format,
				map[string][]byte{
					productExecutableName(): []byte("synthetic ARAM executable"),
					"BUILD-INFO.txt":        []byte("synthetic build"),
				},
			)

			executable, err := Install(archivePath)
			if err != nil {
				t.Fatal(err)
			}
			if !regularFile(executable) {
				t.Fatalf("installed executable %q is missing", executable)
			}
			selected, err := CurrentExecutable()
			if err != nil {
				t.Fatal(err)
			}
			if !samePath(selected, executable) {
				t.Fatalf("current executable = %q, want %q", selected, executable)
			}
			again, err := Install(archivePath)
			if err != nil {
				t.Fatal(err)
			}
			if !samePath(again, executable) {
				t.Fatalf("repeat install = %q, want %q", again, executable)
			}
		})
	}
}

func TestInstallRejectsArchiveTraversal(t *testing.T) {
	isolateConfig(t)
	archivePath := filepath.Join(t.TempDir(), "aram.zip")
	createProductArchive(t, archivePath, "zip", map[string][]byte{
		"../outside.txt": []byte("escape"),
	})
	if _, err := Install(archivePath); err == nil {
		t.Fatal("Install accepted a path-traversal entry")
	}
}

func TestCurrentExecutableRejectsMarkerOutsideRuntimeDirectory(t *testing.T) {
	isolateConfig(t)
	outside := filepath.Join(t.TempDir(), productExecutableName())
	if err := os.WriteFile(outside, []byte("outside"), 0o700); err != nil {
		t.Fatal(err)
	}
	marker, err := currentRuntimePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(marker), 0o700); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"executable":` + quoteJSON(outside) + `}`)
	if err := os.WriteFile(marker, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CurrentExecutable(); err == nil {
		t.Fatal("CurrentExecutable accepted a marker outside the runtime directory")
	}
}

func TestShouldForwardOnlyOriginalUnchangedLauncher(t *testing.T) {
	marker := currentRuntime{
		Executable:         filepath.Join("runtime", productExecutableName()),
		LauncherExecutable: filepath.Join("download", productExecutableName()),
		LauncherSHA256:     "launcher-digest",
	}
	installed := marker.Executable
	if !shouldForward(
		marker,
		marker.LauncherExecutable,
		marker.LauncherSHA256,
		installed,
	) {
		t.Fatal("unchanged original launcher did not forward")
	}
	if shouldForward(marker, marker.LauncherExecutable, "new-build", installed) {
		t.Fatal("a newer executable at the launcher path forwarded to the old runtime")
	}
	if shouldForward(
		marker,
		filepath.Join("new-download", productExecutableName()),
		"new-build",
		installed,
	) {
		t.Fatal("a separately downloaded executable forwarded to the old runtime")
	}
	if shouldForward(marker, installed, "installed-build", installed) {
		t.Fatal("the installed runtime forwarded to itself")
	}
}

func createProductArchive(
	t *testing.T,
	archivePath string,
	format string,
	files map[string][]byte,
) {
	t.Helper()
	switch format {
	case "zip":
		file, err := os.Create(archivePath)
		if err != nil {
			t.Fatal(err)
		}
		writer := zip.NewWriter(file)
		for name, data := range files {
			header := &zip.FileHeader{Name: name, Method: zip.Deflate}
			header.SetMode(0o755)
			entry, err := writer.CreateHeader(header)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := entry.Write(data); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	case "tar.gz":
		var output bytes.Buffer
		compressed := gzip.NewWriter(&output)
		writer := tar.NewWriter(compressed)
		for name, data := range files {
			if err := writer.WriteHeader(&tar.Header{
				Name: name,
				Mode: 0o755,
				Size: int64(len(data)),
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := writer.Write(data); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		if err := compressed.Close(); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(archivePath, output.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unsupported test archive format %q", format)
	}
}

func quoteJSON(value string) string {
	var output bytes.Buffer
	output.WriteByte('"')
	for _, character := range value {
		switch character {
		case '\\', '"':
			output.WriteByte('\\')
			output.WriteRune(character)
		default:
			output.WriteRune(character)
		}
	}
	output.WriteByte('"')
	return output.String()
}
