package integration

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mirusu400/aram-core/cheat"
	"github.com/mirusu400/aram-frontend/frontend"
)

// openSyntheticCheatBackend loads a synthetic application and returns the
// backend together with the address the catalog fixtures patch.
func openSyntheticCheatBackend(t *testing.T) (*Backend, frontend.InputInfo) {
	t.Helper()
	return openSyntheticCheatBackendWithPadding(t, 0)
}

// openSyntheticCheatBackendWithPadding prefixes the container with padding so a
// caller can produce a different file that carries the very same image, the way
// re-archiving a package does.
func openSyntheticCheatBackendWithPadding(
	t *testing.T,
	padding int,
) (*Backend, frontend.InputInfo) {
	t.Helper()
	container := append(make([]byte, padding), syntheticEADS()...)
	path := filepath.Join(t.TempDir(), "synthetic.dat")
	if err := os.WriteFile(path, container, 0o600); err != nil {
		t.Fatal(err)
	}
	backend := NewBackend(nil)
	t.Cleanup(func() { _ = backend.Close() })
	backend.cheatStore.cacheRoot = t.TempDir()

	info, err := backend.Open(context.Background(), frontend.OpenRequest{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	return backend, info
}

// imageIdentity is the hash published catalogs are keyed on.
func imageIdentity(t *testing.T, backend *Backend) string {
	t.Helper()
	library, unavailable := backend.cheatLibrary()
	if library == nil {
		t.Fatalf("no cheat library was attached: %s", unavailable)
	}
	identity := library.Engine().ImageSHA256()
	if identity == "" {
		t.Fatal("the loaded application has no image identity")
	}
	return identity
}

// cheatPatchTarget picks a writable address and reports the bytes a catalog
// must declare as the expected original.
func cheatPatchTarget(t *testing.T, backend *Backend) (uint32, []byte) {
	t.Helper()
	library, unavailable := backend.cheatLibrary()
	if library == nil {
		t.Fatalf("no cheat library was attached: %s", unavailable)
	}
	for _, region := range library.Engine().Regions() {
		if !region.Writable {
			continue
		}
		original, err := library.Engine().ReadBytes(region.Start, 4)
		if err != nil {
			continue
		}
		return region.Start, original
	}
	t.Fatal("no writable cheat region was configured")
	return 0, nil
}

func cheatCatalogDocument(
	t *testing.T,
	sha256 string,
	address uint32,
	expected []byte,
) []byte {
	t.Helper()
	document := fmt.Sprintf(`{
  "version": 3,
  "title": {"image_sha256": %q, "name": "Synthetic Title"},
  "cheats": [{
    "id": "skip-server-authentication",
    "name": "Skip server authentication",
    "description": "Branch past the network check.",
    "category": "bypass",
    "restore_on_disable": true,
    "patches": [{"address": "0x%08x", "value": "aabbccdd", "expected": %q}]
  }]
}`, sha256, address, hex.EncodeToString(expected))
	if _, err := cheat.ParseCatalog([]byte(document)); err != nil {
		t.Fatal(err)
	}
	return []byte(document)
}

func TestOpenAttachesTheCheatEngineToTheLoadedTitle(t *testing.T) {
	backend, info := openSyntheticCheatBackend(t)
	library, unavailable := backend.cheatLibrary()
	if library == nil {
		t.Fatalf("no cheat library was attached: %s", unavailable)
	}
	if library.Engine().TargetSHA256() != info.SHA256 {
		t.Fatalf(
			"cheat target = %q, input = %q",
			library.Engine().TargetSHA256(),
			info.SHA256,
		)
	}
	if len(library.Engine().Regions()) == 0 {
		t.Fatal("the attached cheat engine has no regions")
	}
}

func TestCheatPanelListsAndTogglesPublishedCheats(t *testing.T) {
	backend, _ := openSyntheticCheatBackend(t)
	address, original := cheatPatchTarget(t, backend)
	image := imageIdentity(t, backend)
	document := cheatCatalogDocument(t, image, address, original)

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, request *http.Request) {
			requests++
			if request.URL.Path != "/titles/"+image+".json" {
				writer.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = writer.Write(document)
		},
	))
	t.Cleanup(server.Close)
	backend.cheatStore.baseURL = server.URL

	snapshot, err := backend.ToolSnapshot(context.Background(), frontend.ToolCheats)
	if err != nil {
		t.Fatal(err)
	}
	toggle := snapshot.Fields[0]
	if len(snapshot.Fields) != 1 ||
		!toggle.Checkbox ||
		toggle.Action != cheatActionToggle ||
		toggle.ID != cheatFieldPrefix+"skip-server-authentication" ||
		toggle.Value != "false" ||
		toggle.Detail == "" {
		t.Fatalf("cheat panel fields = %+v", snapshot.Fields)
	}
	if len(snapshot.Actions) != 1 || snapshot.Actions[0].ID != cheatActionRefresh {
		t.Fatalf("cheat panel actions = %+v", snapshot.Actions)
	}
	// A cheat is toggled mid-play, so the panel must not swallow the keypress
	// that advances the game.
	if !snapshot.AllowGuestInput {
		t.Fatal("the cheat panel captures guest input")
	}

	library, _ := backend.cheatLibrary()
	if _, err := backend.ExecuteToolAction(context.Background(), frontend.ToolRequest{
		Kind:   frontend.ToolCheats,
		Action: cheatActionToggle,
		Fields: map[string]string{
			cheatFieldPrefix + "skip-server-authentication": "true",
		},
	}); err != nil {
		t.Fatal(err)
	}
	patched, err := library.Engine().ReadBytes(address, 4)
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(patched) != "aabbccdd" {
		t.Fatalf("guest bytes after enable = %x", patched)
	}
	if entry, ok := library.Entry("skip-server-authentication"); !ok || !entry.Enabled {
		t.Fatalf("library entry after enable = %+v", entry)
	}

	if _, err := backend.ExecuteToolAction(context.Background(), frontend.ToolRequest{
		Kind:   frontend.ToolCheats,
		Action: cheatActionToggle,
		Fields: map[string]string{
			cheatFieldPrefix + "skip-server-authentication": "false",
		},
	}); err != nil {
		t.Fatal(err)
	}
	restored, err := library.Engine().ReadBytes(address, 4)
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(restored) != hex.EncodeToString(original) {
		t.Fatalf("guest bytes after disable = %x, want %x", restored, original)
	}
	if requests != 1 {
		t.Fatalf("cheat database requests = %d, want a single fetch", requests)
	}
}

func TestCheatCatalogIsCachedAndRefreshedOnDemand(t *testing.T) {
	backend, _ := openSyntheticCheatBackend(t)
	address, original := cheatPatchTarget(t, backend)
	image := imageIdentity(t, backend)
	document := cheatCatalogDocument(t, image, address, original)

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, _ *http.Request) {
			requests++
			_, _ = writer.Write(document)
		},
	))
	t.Cleanup(server.Close)
	backend.cheatStore.baseURL = server.URL

	if _, err := backend.ToolSnapshot(context.Background(), frontend.ToolCheats); err != nil {
		t.Fatal(err)
	}
	cachePath, err := backend.cheatStore.cachePath(image)
	if err != nil {
		t.Fatal(err)
	}
	cached, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	var decoded cheat.Catalog
	if err := json.Unmarshal(cached, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Title.ImageSHA256 != image {
		t.Fatalf("cached catalog = %+v", decoded.Title)
	}

	if _, err := backend.ExecuteToolAction(context.Background(), frontend.ToolRequest{
		Kind:   frontend.ToolCheats,
		Action: cheatActionRefresh,
	}); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("cheat database requests = %d, want the refresh to re-fetch", requests)
	}
}

func TestCheatPanelReportsTitlesWithoutPublishedCheats(t *testing.T) {
	backend, _ := openSyntheticCheatBackend(t)
	server := httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusNotFound)
		},
	))
	t.Cleanup(server.Close)
	backend.cheatStore.baseURL = server.URL

	snapshot, err := backend.ToolSnapshot(context.Background(), frontend.ToolCheats)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(snapshot.Lines, "\n")
	if !strings.Contains(joined, "publishes no cheats") {
		t.Fatalf("cheat panel lines = %q", snapshot.Lines)
	}
	if len(snapshot.Actions) != 1 || snapshot.Actions[0].ID != cheatActionRefresh {
		t.Fatalf("cheat panel actions = %+v", snapshot.Actions)
	}
}

func TestCheatCatalogForAnotherTitleIsRejected(t *testing.T) {
	backend, info := openSyntheticCheatBackend(t)
	_ = info
	address, original := cheatPatchTarget(t, backend)
	document := cheatCatalogDocument(t, strings.Repeat("ab", 32), address, original)

	server := httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write(document)
		},
	))
	t.Cleanup(server.Close)
	backend.cheatStore.baseURL = server.URL

	if _, err := backend.ToolSnapshot(context.Background(), frontend.ToolCheats); err == nil {
		t.Fatalf("a catalog for another title was accepted for %s", info.SHA256)
	}
}

// The point of keying on the image: a container that was re-archived has a
// different file hash but carries the same program, and must keep its cheats.
func TestRepackagedContainerKeepsItsPublishedCheats(t *testing.T) {
	original, originalInfo := openSyntheticCheatBackend(t)
	address, bytesBefore := cheatPatchTarget(t, original)
	image := imageIdentity(t, original)
	document := cheatCatalogDocument(t, image, address, bytesBefore)

	repacked, repackedInfo := openSyntheticCheatBackendWithPadding(t, 64)
	if repackedInfo.SHA256 == originalInfo.SHA256 {
		t.Fatal("the repacked container kept the original file hash")
	}
	if got := imageIdentity(t, repacked); got != image {
		t.Fatalf("repacked image identity = %s, want %s", got, image)
	}

	served := ""
	server := httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, request *http.Request) {
			served = request.URL.Path
			if request.URL.Path != "/titles/"+image+".json" {
				writer.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = writer.Write(document)
		},
	))
	t.Cleanup(server.Close)
	repacked.cheatStore.baseURL = server.URL

	snapshot, err := repacked.ToolSnapshot(context.Background(), frontend.ToolCheats)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Fields) != 1 ||
		snapshot.Fields[0].Label != "Skip server authentication" {
		t.Fatalf("cheat panel fields = %+v (served %s)", snapshot.Fields, served)
	}
}

// Entries published before an image identity was known are still reachable
// through the container hash.
func TestCatalogPublishedUnderTheFileHashStillResolves(t *testing.T) {
	backend, info := openSyntheticCheatBackend(t)
	address, original := cheatPatchTarget(t, backend)
	image := imageIdentity(t, backend)
	document := cheatCatalogDocument(t, info.SHA256, address, original)

	var requested []string
	server := httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, request *http.Request) {
			requested = append(requested, request.URL.Path)
			if request.URL.Path != "/titles/"+info.SHA256+".json" {
				writer.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = writer.Write(document)
		},
	))
	t.Cleanup(server.Close)
	backend.cheatStore.baseURL = server.URL

	snapshot, err := backend.ToolSnapshot(context.Background(), frontend.ToolCheats)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Fields) != 1 ||
		snapshot.Fields[0].Label != "Skip server authentication" {
		t.Fatalf("cheat panel fields = %+v", snapshot.Fields)
	}
	if len(requested) != 2 || requested[0] != "/titles/"+image+".json" {
		t.Fatalf("requests = %v, want the image identity tried first", requested)
	}
}

func defaultOnCatalogDocument(
	t *testing.T,
	sha256 string,
	address uint32,
	expected []byte,
) []byte {
	t.Helper()
	document := fmt.Sprintf(`{
  "version": 3,
  "title": {"image_sha256": %q, "name": "Synthetic Title"},
  "cheats": [{
    "id": "skip-server-authentication",
    "name": "Skip server authentication",
    "description": "The server stopped answering years ago.",
    "category": "bypass",
    "restore_on_disable": true,
    "default_enabled": true,
    "patches": [{"address": "0x%08x", "value": "aabbccdd", "expected": %q}]
  }]
}`, sha256, address, hex.EncodeToString(expected))
	if _, err := cheat.ParseCatalog([]byte(document)); err != nil {
		t.Fatal(err)
	}
	return []byte(document)
}

// A title that cannot run unmodified should come up working, without anyone
// having to find the repair first.
func TestDefaultEnabledCheatAppliesWhenTheTitleLoads(t *testing.T) {
	backend, _ := openSyntheticCheatBackend(t)
	address, original := cheatPatchTarget(t, backend)
	image := imageIdentity(t, backend)
	backend.cheatStore.localDir = writeLocalCatalog(
		t,
		image,
		defaultOnCatalogDocument(t, image, address, original),
	)

	snapshot, err := backend.ToolSnapshot(context.Background(), frontend.ToolCheats)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Fields) != 1 || snapshot.Fields[0].Value != "true" {
		t.Fatalf("default-on cheat is off in the panel: %+v", snapshot.Fields)
	}
	library, _ := backend.cheatLibrary()
	patched, err := library.Engine().ReadBytes(address, 4)
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(patched) != "aabbccdd" {
		t.Fatalf("guest bytes with a default-on cheat = %x", patched)
	}
}

// The shell starts the guest the moment Open returns, so a default-on repair
// has to be in guest memory by then — a boot-time patch applied when the
// panel first opens has already missed the code it repairs. This is the
// Zenonia 1 case: the authentication bypass must precede the first boot.
func TestDefaultEnabledCheatIsAppliedBeforeOpenReturns(t *testing.T) {
	probe, _ := openSyntheticCheatBackend(t)
	address, original := cheatPatchTarget(t, probe)
	image := imageIdentity(t, probe)
	catalogDir := writeLocalCatalog(
		t,
		image,
		defaultOnCatalogDocument(t, image, address, original),
	)
	_ = probe.Close()

	path := filepath.Join(t.TempDir(), "synthetic.dat")
	if err := os.WriteFile(path, syntheticEADS(), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := NewBackend(nil)
	t.Cleanup(func() { _ = backend.Close() })
	backend.cheatStore.cacheRoot = t.TempDir()
	backend.cheatStore.localDir = catalogDir
	if _, err := backend.Open(context.Background(), frontend.OpenRequest{Path: path}); err != nil {
		t.Fatal(err)
	}

	// No panel was opened: the repair must already be in guest memory.
	library, unavailable := backend.cheatLibrary()
	if library == nil {
		t.Fatalf("no cheat library was attached: %s", unavailable)
	}
	patched, err := library.Engine().ReadBytes(address, 4)
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(patched) != "aabbccdd" {
		t.Fatalf("guest bytes after open = %x, want the default applied", patched)
	}
	if entry, ok := library.Entry("skip-server-authentication"); !ok || !entry.Enabled {
		t.Fatalf("library entry after open = %+v", entry)
	}
}

// Turning a default off has to stick, or the choice would be undone by every
// later launch.
func TestTurningOffADefaultIsRememberedAcrossOpens(t *testing.T) {
	shared := t.TempDir()
	catalogDir := ""

	// The catalog directory is known from the second open on, so later opens
	// resolve it while loading, the way a real launch does.
	openOnce := func() (*Backend, uint32, []byte) {
		path := filepath.Join(t.TempDir(), "synthetic.dat")
		if err := os.WriteFile(path, syntheticEADS(), 0o600); err != nil {
			t.Fatal(err)
		}
		backend := NewBackend(nil)
		t.Cleanup(func() { _ = backend.Close() })
		backend.cheatStore.cacheRoot = shared
		backend.cheatStore.localDir = catalogDir
		if _, err := backend.Open(context.Background(), frontend.OpenRequest{Path: path}); err != nil {
			t.Fatal(err)
		}
		address, original := cheatPatchTarget(t, backend)
		image := imageIdentity(t, backend)
		if catalogDir == "" {
			catalogDir = writeLocalCatalog(
				t,
				image,
				defaultOnCatalogDocument(t, image, address, original),
			)
			backend.cheatStore.localDir = catalogDir
		}
		return backend, address, original
	}

	first, address, original := openOnce()
	if _, err := first.ToolSnapshot(context.Background(), frontend.ToolCheats); err != nil {
		t.Fatal(err)
	}
	if _, err := first.ExecuteToolAction(context.Background(), frontend.ToolRequest{
		Kind:   frontend.ToolCheats,
		Action: cheatActionToggle,
		Fields: map[string]string{
			cheatFieldPrefix + "skip-server-authentication": "false",
		},
	}); err != nil {
		t.Fatal(err)
	}
	_ = first.Close()

	second, _, _ := openOnce()
	snapshot, err := second.ToolSnapshot(context.Background(), frontend.ToolCheats)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Fields) != 1 || snapshot.Fields[0].Value != "false" {
		t.Fatalf("the default came back after being turned off: %+v", snapshot.Fields)
	}
	library, _ := second.cheatLibrary()
	current, err := library.Engine().ReadBytes(address, 4)
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(current) != hex.EncodeToString(original) {
		t.Fatalf("guest bytes after declining the default = %x", current)
	}
}

func writeLocalCatalog(t *testing.T, image string, document []byte) string {
	t.Helper()
	root := t.TempDir()
	directory := filepath.Join(root, "titles")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, image+".json")
	if err := os.WriteFile(path, document, 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestCheatDatabaseURLMustUseHTTPS(t *testing.T) {
	store := &cheatCatalogStore{baseURL: "http://cheats.example.com"}
	if _, err := store.titleURL(strings.Repeat("ab", 32)); err == nil {
		t.Fatal("a plaintext cheat database host was accepted")
	}
	store.baseURL = "https://cheats.example.com"
	endpoint, err := store.titleURL(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(endpoint, "/titles/"+strings.Repeat("ab", 32)+".json") {
		t.Fatalf("cheat database endpoint = %q", endpoint)
	}
	if _, err := store.titleURL("../../etc/passwd"); err == nil {
		t.Fatal("a traversal title identity was accepted")
	}
}
