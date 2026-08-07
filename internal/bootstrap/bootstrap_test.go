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

func TestShouldForwardRecognizesTheLauncherByItsContents(t *testing.T) {
	root := filepath.Join(t.TempDir(), "versions")
	const shipped = "shipped build"
	installed := writeExecutable(t, root, "selected", productExecutableName(), "installed build")
	superseded := writeExecutable(t, root, "superseded", productExecutableName(), shipped)
	downloads := t.TempDir()
	launcher := writeExecutable(t, downloads, "aram-macos-arm64", productExecutableName(), shipped)
	marker := currentRuntime{
		Executable:         installed,
		LauncherExecutable: launcher,
		LauncherSHA256:     digestOf(t, launcher),
	}

	for _, testcase := range []struct {
		name    string
		current string
		forward bool
	}{
		{"unchanged launcher", launcher, true},
		{
			"launcher relocated after it was recorded",
			writeExecutable(t, t.TempDir(), "Applications", productExecutableName(), shipped),
			true,
		},
		{
			"a newer build than the one recorded as the launcher",
			writeExecutable(t, downloads, "newer", productExecutableName(), "later build"),
			false,
		},
		{
			"unrelated build elsewhere",
			writeExecutable(t, t.TempDir(), "work", productExecutableName(), "developer build"),
			false,
		},
		{"the selected runtime itself", installed, false},
		{
			"the selected runtime running from inside its bundle",
			writeExecutable(
				t,
				filepath.Join(root, "selected"),
				filepath.Join("ARAM.app", "Contents", "MacOS"),
				productExecutableName(),
				shipped,
			),
			false,
		},
		{"a superseded runtime", superseded, true},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			forward, err := shouldForward(marker, root, testcase.current, installed)
			if err != nil {
				t.Fatal(err)
			}
			if forward != testcase.forward {
				t.Fatalf("forward = %t, want %t", forward, testcase.forward)
			}
		})
	}
}

func TestInstallRerecordsALauncherThatMoved(t *testing.T) {
	isolateConfig(t)
	markerPath, err := currentRuntimePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(markerPath), 0o700); err != nil {
		t.Fatal(err)
	}
	stale := []byte(`{"executable":"","launcher_executable":` +
		quoteJSON(filepath.Join("gone", productExecutableName())) +
		`,"launcher_sha256":"0bsolete"}`)
	if err := os.WriteFile(markerPath, stale, 0o600); err != nil {
		t.Fatal(err)
	}

	archivePath := filepath.Join(t.TempDir(), "aram.tar.gz")
	createProductArchive(t, archivePath, "tar.gz", map[string][]byte{
		productExecutableName(): []byte("synthetic ARAM executable"),
	})
	if _, err := Install(archivePath); err != nil {
		t.Fatal(err)
	}

	marker, err := readCurrentRuntime()
	if err != nil {
		t.Fatal(err)
	}
	// The test binary stands in for the launcher a person started, and it is
	// not inside the isolated runtime directory.
	running, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if !samePath(marker.LauncherExecutable, running) {
		t.Fatalf(
			"launcher = %q, want the running executable %q",
			marker.LauncherExecutable,
			running,
		)
	}
	if marker.LauncherSHA256 != digestOf(t, running) {
		t.Fatalf("launcher digest = %q, want the running one", marker.LauncherSHA256)
	}
}

func writeExecutable(t *testing.T, root string, directory string, name string, contents string) string {
	t.Helper()
	folder := filepath.Join(root, directory)
	if err := os.MkdirAll(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(folder, name)
	if err := os.WriteFile(path, []byte(contents), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func digestOf(t *testing.T, path string) string {
	t.Helper()
	digest, err := fileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	return digest
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
