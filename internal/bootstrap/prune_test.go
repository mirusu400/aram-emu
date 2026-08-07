package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPruneRuntimesKeepsSelectedAndOnePrevious(t *testing.T) {
	isolateConfig(t)
	selected := installSyntheticRuntime(t, "selected build")
	root := filepath.Dir(filepath.Dir(selected))

	previous := writeRuntime(t, root, "previous", time.Now().Add(-time.Hour))
	older := writeRuntime(t, root, "older", time.Now().Add(-24*time.Hour))
	oldest := writeRuntime(t, root, "oldest", time.Now().Add(-72*time.Hour))

	if err := PruneRuntimes(1); err != nil {
		t.Fatal(err)
	}
	if !regularFile(selected) {
		t.Fatal("prune removed the selected runtime")
	}
	if _, err := os.Stat(previous); err != nil {
		t.Fatalf("prune removed the most recent previous runtime: %v", err)
	}
	for _, discarded := range []string{older, oldest} {
		if _, err := os.Stat(discarded); !os.IsNotExist(err) {
			t.Fatalf("prune kept superseded runtime %q: %v", discarded, err)
		}
	}
	current, err := CurrentExecutable()
	if err != nil {
		t.Fatal(err)
	}
	if !samePath(current, selected) {
		t.Fatalf("current executable = %q, want %q", current, selected)
	}
}

func TestPruneRuntimesKeepingNoneLeavesOnlyTheSelectedRuntime(t *testing.T) {
	isolateConfig(t)
	selected := installSyntheticRuntime(t, "selected build")
	root := filepath.Dir(filepath.Dir(selected))
	previous := writeRuntime(t, root, "previous", time.Now().Add(-time.Hour))

	if err := PruneRuntimes(0); err != nil {
		t.Fatal(err)
	}
	if !regularFile(selected) {
		t.Fatal("prune removed the selected runtime")
	}
	if _, err := os.Stat(previous); !os.IsNotExist(err) {
		t.Fatalf("prune kept the previous runtime: %v", err)
	}
}

func TestPruneRuntimesClearsAbandonedStagingAndInterruptedDeletions(t *testing.T) {
	isolateConfig(t)
	selected := installSyntheticRuntime(t, "selected build")
	root := filepath.Dir(filepath.Dir(selected))

	abandoned := writeRuntime(
		t,
		root,
		runtimeStagingPrefix+"crashed",
		time.Now().Add(-48*time.Hour),
	)
	active := writeRuntime(t, root, runtimeStagingPrefix+"running", time.Now())
	interrupted := writeRuntime(
		t,
		root,
		runtimeTrashPrefix+"halfdeleted",
		time.Now(),
	)

	if err := PruneRuntimes(1); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(abandoned); !os.IsNotExist(err) {
		t.Fatalf("prune kept an abandoned staging directory: %v", err)
	}
	if _, err := os.Stat(interrupted); !os.IsNotExist(err) {
		t.Fatalf("prune kept an interrupted deletion: %v", err)
	}
	if _, err := os.Stat(active); err != nil {
		t.Fatalf("prune removed an installation in progress: %v", err)
	}
}

func TestPruneRuntimesWithoutInstalledRuntimes(t *testing.T) {
	isolateConfig(t)
	if err := PruneRuntimes(1); err != nil {
		t.Fatalf("prune before the first install: %v", err)
	}
}

func TestRuntimeOwningResolvesBundledExecutable(t *testing.T) {
	root := filepath.Join("runtime", "versions")
	bundled := filepath.Join(
		root,
		"2445503b4be3b555",
		"ARAM.app",
		"Contents",
		"MacOS",
		"aram",
	)
	name, ok := runtimeOwning(root, bundled)
	if !ok || name != "2445503b4be3b555" {
		t.Fatalf("runtimeOwning(%q) = %q, %t", bundled, name, ok)
	}
	if _, ok := runtimeOwning(root, filepath.Join("elsewhere", "aram")); ok {
		t.Fatal("runtimeOwning claimed a path outside the runtime directory")
	}
	if _, ok := runtimeOwning(root, root); ok {
		t.Fatal("runtimeOwning claimed the runtime directory itself")
	}
}

// installSyntheticRuntime installs a one-file product archive and returns the
// executable the marker selects.
func installSyntheticRuntime(t *testing.T, contents string) string {
	t.Helper()
	archivePath := filepath.Join(t.TempDir(), "aram.tar.gz")
	createProductArchive(t, archivePath, "tar.gz", map[string][]byte{
		productExecutableName(): []byte(contents),
		"BUILD-INFO.txt":        []byte(contents),
	})
	executable, err := Install(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	return executable
}

// writeRuntime creates a runtime directory installed at the given time and
// returns the path of the executable inside it.
func writeRuntime(
	t *testing.T,
	root string,
	name string,
	installed time.Time,
) string {
	t.Helper()
	directory := filepath.Join(root, name)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(directory, productExecutableName())
	if err := os.WriteFile(executable, []byte(name), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(directory, installed, installed); err != nil {
		t.Fatal(err)
	}
	return executable
}
