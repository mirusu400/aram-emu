package integration

import (
	"bytes"
	"context"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mirusu400/aram-frontend/frontend"
)

// Issue 512 kept using a superseded authentication repair from the cache.
// Its replacement must reach guest memory before the title starts executing.
func TestStaleCheatCatalogIsUpdatedBeforeOpenReturns(t *testing.T) {
	probe, _ := openSyntheticCheatBackend(t)
	address, original := cheatPatchTarget(t, probe)
	identity := imageIdentity(t, probe)
	oldDocument := defaultOnCatalogDocument(t, identity, address, original)
	newDocument := bytes.ReplaceAll(oldDocument, []byte("aabbccdd"), []byte("11223344"))
	_ = probe.Close()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/titles/"+identity+".json" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(newDocument)
	}))
	t.Cleanup(server.Close)
	backend := NewBackend(nil)
	t.Cleanup(func() { _ = backend.Close() })
	backend.cheatStore.cacheRoot = t.TempDir()
	backend.cheatStore.baseURL = server.URL
	backend.cheatStore.writeCache(identity, oldDocument)
	cachePath, err := backend.cheatStore.cachePath(identity)
	if err != nil {
		t.Fatal(err)
	}
	expired := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(cachePath, expired, expired); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "synthetic.dat")
	if err := os.WriteFile(path, syntheticEADS(), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Open(context.Background(), frontend.OpenRequest{Path: path}); err != nil {
		t.Fatal(err)
	}
	library, unavailable := backend.cheatLibrary()
	if library == nil {
		t.Fatalf("no cheat library: %s", unavailable)
	}
	patched, err := library.Engine().ReadBytes(address, 4)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(patched); got != "11223344" {
		t.Fatalf("guest bytes after open = %s, want refreshed repair 11223344", got)
	}
	if requests.Load() != 1 {
		t.Fatalf("refresh requests = %d, want 1", requests.Load())
	}
	cached, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(cached, newDocument) {
		t.Fatal("refreshed catalog was not cached")
	}
}

func TestCheatCatalogCacheKeepsItsLastValidCopy(t *testing.T) {
	for _, test := range []struct {
		name       string
		stale      bool
		status     int
		wrongTitle bool
	}{
		{name: "fresh", status: http.StatusInternalServerError},
		{name: "offline", stale: true, status: http.StatusServiceUnavailable},
		{name: "unpublished", stale: true, status: http.StatusNotFound},
		{name: "wrong title", stale: true, status: http.StatusOK, wrongTitle: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			identity := strings.Repeat("ab", 32)
			document := defaultOnCatalogDocument(t, identity, 0x1000, []byte{1, 2, 3, 4})
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.WriteHeader(test.status)
				if test.wrongTitle {
					_, _ = w.Write(bytes.ReplaceAll(document, []byte(identity), []byte(strings.Repeat("cd", 32))))
				}
			}))
			t.Cleanup(server.Close)
			store := newCheatCatalogStore()
			store.cacheRoot = t.TempDir()
			store.baseURL = server.URL
			store.writeCache(identity, document)
			cachePath, err := store.cachePath(identity)
			if err != nil {
				t.Fatal(err)
			}
			if test.stale {
				expired := time.Now().Add(-48 * time.Hour)
				if err := os.Chtimes(cachePath, expired, expired); err != nil {
					t.Fatal(err)
				}
			}
			catalog, source, err := store.load(context.Background(), []string{identity})
			if err != nil {
				t.Fatal(err)
			}
			if source != "cache" || catalog.Title.ImageSHA256 != identity {
				t.Fatalf("cached result: source=%q identity=%q", source, catalog.Title.ImageSHA256)
			}
			wantRequests := int32(0)
			if test.stale {
				wantRequests = 1
			}
			if requests.Load() != wantRequests {
				t.Fatalf("requests = %d, want %d", requests.Load(), wantRequests)
			}
			cached, err := os.ReadFile(cachePath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(cached, document) {
				t.Fatal("failed refresh changed the cached catalog")
			}
		})
	}
}
