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
	path := filepath.Join(t.TempDir(), "synthetic.dat")
	if err := os.WriteFile(path, syntheticEADS(), 0o600); err != nil {
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
  "version": 1,
  "title": {"sha256": %q, "name": "Synthetic Title"},
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
	backend, info := openSyntheticCheatBackend(t)
	address, original := cheatPatchTarget(t, backend)
	document := cheatCatalogDocument(t, info.SHA256, address, original)

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, request *http.Request) {
			requests++
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
	if !strings.Contains(strings.Join(snapshot.Lines, "\n"), "Skip server authentication") {
		t.Fatalf("cheat panel lines = %q", snapshot.Lines)
	}
	if len(snapshot.Fields) != 1 || len(snapshot.Fields[0].Options) != 1 {
		t.Fatalf("cheat panel fields = %+v", snapshot.Fields)
	}
	if len(snapshot.Actions) != 3 {
		t.Fatalf("cheat panel actions = %+v", snapshot.Actions)
	}

	library, _ := backend.cheatLibrary()
	if _, err := backend.ExecuteToolAction(context.Background(), frontend.ToolRequest{
		Kind:   frontend.ToolCheats,
		Action: cheatActionEnable,
		Fields: map[string]string{cheatFieldCheat: "skip-server-authentication"},
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
		Action: cheatActionDisable,
		Fields: map[string]string{cheatFieldCheat: "skip-server-authentication"},
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
	backend, info := openSyntheticCheatBackend(t)
	address, original := cheatPatchTarget(t, backend)
	document := cheatCatalogDocument(t, info.SHA256, address, original)

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
	cachePath, err := backend.cheatStore.cachePath(info.SHA256)
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
	if decoded.Title.SHA256 != info.SHA256 {
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
