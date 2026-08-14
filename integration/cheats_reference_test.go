package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mirusu400/aram-core/cheat"
	"github.com/mirusu400/aram-frontend/frontend"
)

// zenoniaSHA256 identifies the LGT 제노니아 1 package from aram-emu#4, whose
// authentication bypass the cheat database publishes.
const zenoniaSHA256 = "3cc7a9b4cb15818cdd5a66f7e520c7b9b36f1df8d2df096aafa961b1cb2b682c"

// TestPublishedCheatsApplyToTheirReferenceTitle loads a title the cheat
// database publishes for and applies every catalog entry against the real
// image. A published patch whose expected bytes drifted from the loaded
// application fails here rather than in a user's hands.
func TestPublishedCheatsApplyToTheirReferenceTitle(t *testing.T) {
	root := os.Getenv("ARAM_TEST_DATA")
	if root == "" {
		t.Skip("ARAM_TEST_DATA is not set")
	}
	catalogRoot := os.Getenv(cheatDirectoryEnv)
	if catalogRoot == "" {
		t.Skipf("%s is not set to an aram-cheat checkout", cheatDirectoryEnv)
	}
	path := findTitleByHash(t, root, zenoniaSHA256)

	temporary := t.TempDir()
	t.Setenv("APPDATA", temporary)
	t.Setenv("XDG_CONFIG_HOME", temporary)

	backend := NewBackend(nil)
	t.Cleanup(func() { _ = backend.Close() })
	backend.cheatStore.cacheRoot = t.TempDir()
	backend.cheatStore.localDir = catalogRoot
	// A published catalog must satisfy the panel without any network access.
	backend.cheatStore.baseURL = "https://cheat-database.invalid"

	info, err := backend.Open(context.Background(), frontend.OpenRequest{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if info.SHA256 != zenoniaSHA256 {
		t.Fatalf("input SHA-256 = %s, want %s", info.SHA256, zenoniaSHA256)
	}

	snapshot, err := backend.ToolSnapshot(context.Background(), frontend.ToolCheats)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(snapshot.Lines, "\n"), "Catalog: local directory") {
		t.Fatalf("cheat panel lines = %q", snapshot.Lines)
	}

	library, unavailable := backend.cheatLibrary()
	if library == nil {
		t.Fatalf("no cheat library was attached: %s", unavailable)
	}
	entries := library.Entries()
	if len(entries) == 0 {
		t.Fatalf("the cheat database publishes nothing for %s", zenoniaSHA256)
	}
	// Opening applied the catalog defaults, so a repair the title cannot run
	// without is already in guest memory — the Zenonia 1 bug was exactly this
	// patch waiting for the panel and missing the first boot.
	for _, entry := range entries {
		if entry.Cheat.DefaultEnabled && !entry.Enabled {
			t.Fatalf("default-enabled cheat %s did not apply at open", entry.Cheat.ID)
		}
	}
	for _, entry := range entries {
		assertCheatAppliesAndReverts(t, library, entry)
	}
}

func assertCheatAppliesAndReverts(t *testing.T, library *cheat.Library, applied cheat.Entry) {
	t.Helper()
	engine := library.Engine()
	entry := applied.Cheat
	if applied.Enabled {
		// The open already applied this cheat; prove it landed, then restore
		// the original bytes so the roundtrip below starts from the image.
		for index, patch := range entry.Patches {
			current, err := engine.ReadBytes(uint32(patch.Address), len(patch.Value))
			if err != nil {
				t.Fatalf("%s patch %d: %v", entry.ID, index, err)
			}
			if !bytes.Equal(current, patch.Value) {
				t.Fatalf(
					"%s patch %d at 0x%08x applied at open = %x, want %x",
					entry.ID,
					index,
					uint32(patch.Address),
					current,
					patch.Value,
				)
			}
		}
		if !entry.RestoreOnDisable {
			return
		}
		if err := library.SetEnabled(entry.ID, false); err != nil {
			t.Fatalf("disable %s: %v", entry.ID, err)
		}
	}
	for index, patch := range entry.Patches {
		original, err := engine.ReadBytes(uint32(patch.Address), len(patch.Expected))
		if err != nil {
			t.Fatalf("%s patch %d: %v", entry.ID, index, err)
		}
		if !bytes.Equal(original, patch.Expected) {
			t.Fatalf(
				"%s patch %d at 0x%08x: image holds %x, catalog expects %x",
				entry.ID,
				index,
				uint32(patch.Address),
				original,
				patch.Expected,
			)
		}
	}

	if err := library.SetEnabled(entry.ID, true); err != nil {
		t.Fatalf("enable %s: %v", entry.ID, err)
	}
	for index, patch := range entry.Patches {
		applied, err := engine.ReadBytes(uint32(patch.Address), len(patch.Value))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(applied, patch.Value) {
			t.Fatalf(
				"%s patch %d at 0x%08x after enable = %x, want %x",
				entry.ID,
				index,
				uint32(patch.Address),
				applied,
				patch.Value,
			)
		}
	}

	if !entry.RestoreOnDisable {
		return
	}
	if err := library.SetEnabled(entry.ID, false); err != nil {
		t.Fatalf("disable %s: %v", entry.ID, err)
	}
	for index, patch := range entry.Patches {
		restored, err := engine.ReadBytes(uint32(patch.Address), len(patch.Expected))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(restored, patch.Expected) {
			t.Fatalf(
				"%s patch %d at 0x%08x after disable = %x, want %x",
				entry.ID,
				index,
				uint32(patch.Address),
				restored,
				patch.Expected,
			)
		}
	}
}

func findTitleByHash(t *testing.T, root, want string) string {
	t.Helper()
	var selected string
	stop := errors.New("title selected")
	err := filepath.WalkDir(root, func(
		path string,
		entry os.DirEntry,
		walkErr error,
	) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".zip") {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		digest := sha256.New()
		if _, err := io.Copy(digest, file); err != nil {
			return err
		}
		if hex.EncodeToString(digest.Sum(nil)) != want {
			return nil
		}
		selected = path
		return stop
	})
	if err != nil && !errors.Is(err, stop) {
		t.Fatal(err)
	}
	if selected == "" {
		t.Skipf("ARAM_TEST_DATA contains no title with SHA-256 %s", want)
	}
	return selected
}
