package remoteinput

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func TestParseWebAndNativeLinks(t *testing.T) {
	digest := strings.Repeat("ab", 32)
	app := "https://downloads.example.invalid/apps/demo%20game.zip#ignored"
	web := "https://aram.mir.sh/player/?ch=nightly&app=" + urlQueryEscape(app) + "&sha256=" + strings.ToUpper(digest)

	spec, ok, err := Parse(web)
	if err != nil || !ok {
		t.Fatalf("Parse(web) = %+v, %t, %v", spec, ok, err)
	}
	if spec.URL != "https://downloads.example.invalid/apps/demo%20game.zip" {
		t.Fatalf("URL = %q", spec.URL)
	}
	if spec.Name != "demo game.zip" || spec.SHA256 != digest {
		t.Fatalf("spec = %+v", spec)
	}

	native := "aram://open?url=" + urlQueryEscape(web)
	wrapped, ok, err := Parse(native)
	if err != nil || !ok || wrapped != spec {
		t.Fatalf("Parse(native) = %+v, %t, %v, want %+v", wrapped, ok, err, spec)
	}

	direct := "aram://open?app=" + urlQueryEscape(app) + "&sha256=" + digest
	got, ok, err := Parse(direct)
	if err != nil || !ok || got != spec {
		t.Fatalf("Parse(direct) = %+v, %t, %v, want %+v", got, ok, err, spec)
	}
}

func TestParseRejectsUnsafeAndAmbiguousLinks(t *testing.T) {
	digest := strings.Repeat("0", 64)
	cases := []string{
		"https://example.invalid/player/?app=https%3A%2F%2Fexample.invalid%2Fa.zip&sha256=" + digest,
		"https://aram.mir.sh/player/?app=http%3A%2F%2Fexample.invalid%2Fa.zip&sha256=" + digest,
		"https://aram.mir.sh/player/?app=https%3A%2F%2Fuser%3Apass%40example.invalid%2Fa.zip&sha256=" + digest,
		"https://aram.mir.sh/player/?app=https%3A%2F%2Fexample.invalid%2Fa.zip",
		"https://aram.mir.sh/player/?app=https%3A%2F%2Fexample.invalid%2Fa.zip&app=https%3A%2F%2Fexample.invalid%2Fb.zip&sha256=" + digest,
		"aram://other?app=https%3A%2F%2Fexample.invalid%2Fa.zip&sha256=" + digest,
	}
	for _, raw := range cases {
		if _, ok, err := Parse(raw); !ok || err == nil {
			t.Errorf("Parse(%q) = ok %t, err %v, want recognized rejection", raw, ok, err)
		}
	}
	if _, ok, err := Parse("game.zip"); err != nil || ok {
		t.Fatalf("ordinary path = ok %t, err %v", ok, err)
	}
}

func TestDownloadPersistsVerifiedPackageAndReusesIt(t *testing.T) {
	payload := []byte("synthetic package")
	digestBytes := sha256.Sum256(payload)
	digest := hex.EncodeToString(digestBytes[:])
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Length", "17")
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	root := t.TempDir()
	spec := Spec{URL: server.URL + "/demo.zip", Name: "demo.zip", SHA256: digest}
	client := server.Client()
	path, err := Download(context.Background(), client, root, spec)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != filepath.Join(root, digest[:2]) {
		t.Fatalf("path = %q", path)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != string(payload) {
		t.Fatalf("saved data = %q, %v", data, err)
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(path); err != nil || info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("saved mode = %v, %v", info, err)
		}
	}

	again, err := Download(context.Background(), client, root, spec)
	if err != nil || again != path {
		t.Fatalf("cached download = %q, %v", again, err)
	}
	if requests.Load() != 1 {
		t.Fatalf("requests = %d, want 1", requests.Load())
	}
}

func TestDownloadRejectsMismatchOversizeAndUnsafeRedirect(t *testing.T) {
	payload := []byte("wrong")
	digest := strings.Repeat("0", 64)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/large":
			w.Header().Set("Content-Length", "999")
		case "/redirect":
			http.Redirect(w, r, "http://example.invalid/package.zip", http.StatusFound)
		default:
			_, _ = w.Write(payload)
		}
	}))
	defer server.Close()

	client := server.Client()
	for _, path := range []string{"/mismatch", "/large", "/redirect"} {
		spec := Spec{URL: server.URL + path, Name: "demo.zip", SHA256: digest}
		_, err := download(context.Background(), client, t.TempDir(), spec, 16)
		if err == nil {
			t.Fatalf("download(%s) succeeded", path)
		}
	}
}

func TestCorruptCachedPackageIsReplaced(t *testing.T) {
	payload := []byte("replacement")
	digestBytes := sha256.Sum256(payload)
	digest := hex.EncodeToString(digestBytes[:])
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	root := t.TempDir()
	spec := Spec{URL: server.URL + "/demo.zip", Name: "demo.zip", SHA256: digest}
	target := packagePath(root, spec)
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	path, err := Download(context.Background(), server.Client(), root, spec)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	got, _ := io.ReadAll(file)
	if string(got) != string(payload) {
		t.Fatalf("replacement = %q", got)
	}
}

func TestDefaultDownloaderRejectsPrivateNetworkTargets(t *testing.T) {
	spec := Spec{
		URL:    "https://127.0.0.1/package.zip",
		Name:   "package.zip",
		SHA256: strings.Repeat("0", 64),
	}
	_, err := Download(context.Background(), nil, t.TempDir(), spec)
	if err == nil || !strings.Contains(err.Error(), "public IP") {
		t.Fatalf("private target error = %v", err)
	}
}

func urlQueryEscape(value string) string {
	replacer := strings.NewReplacer(
		"%", "%25", " ", "%20", ":", "%3A", "/", "%2F", "?", "%3F",
		"&", "%26", "=", "%3D", "#", "%23", "@", "%40",
	)
	return replacer.Replace(value)
}
