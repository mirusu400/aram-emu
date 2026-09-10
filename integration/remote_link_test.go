package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/mirusu400/aram-emu/internal/remoteinput"
	"github.com/mirusu400/aram-frontend/frontend"
)

// TestPlayerPermalinkDownloadsPersistsAndRuns exercises the complete
// application-layer path behind every native deep-link adapter: parse the public
// player URL, download and pin its bytes, persist the package, open it through
// the product backend, and start the resulting machine. Reopening the same link
// must use the verified local copy without another network request.
func TestPlayerPermalinkDownloadsPersistsAndRuns(t *testing.T) {
	data := syntheticEADS()
	digestBytes := sha256.Sum256(data)
	digest := hex.EncodeToString(digestBytes[:])
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		writer.Header().Set("Content-Length", fmt.Sprint(len(data)))
		_, _ = writer.Write(data)
	}))
	client := server.Client()

	link := "https://aram.mir.sh/player/?app=" +
		url.QueryEscape(server.URL+"/synthetic.dat") +
		"&sha256=" + digest
	root := filepath.Join(t.TempDir(), "downloads")
	path, spec, err := remoteinput.Resolve(context.Background(), client, root, link)
	if err != nil {
		t.Fatal(err)
	}
	if spec.SHA256 != digest {
		t.Fatalf("resolved SHA-256 = %q, want %q", spec.SHA256, digest)
	}
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("persistent package stat = %v, %v", info, err)
	}

	backend := NewBackend(nil)
	t.Cleanup(func() { _ = backend.Close() })
	info, err := backend.Open(context.Background(), frontend.OpenRequest{
		Path:           path,
		DisplayName:    spec.Name,
		ExpectedSHA256: spec.SHA256,
	})
	if err != nil {
		t.Fatal(err)
	}
	if info.SHA256 != digest || backend.State() != frontend.StateReady {
		t.Fatalf("opened info/state = %+v / %s", info, backend.State())
	}
	if err := backend.Execute(context.Background(), frontend.CommandStart); err != nil {
		t.Fatal(err)
	}
	if backend.State() != frontend.StateRunning {
		t.Fatalf("state after linked package start = %s", backend.State())
	}

	server.Close()
	cachedPath, _, err := remoteinput.Resolve(context.Background(), client, root, link)
	if err != nil {
		t.Fatalf("reopen verified cache: %v", err)
	}
	if cachedPath != path {
		t.Fatalf("cached path = %q, want %q", cachedPath, path)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("network requests = %d, want 1", got)
	}
}
