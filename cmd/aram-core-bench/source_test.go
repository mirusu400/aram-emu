package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestBenchmarkSourcesReadFileAndLoopbackIdentically(t *testing.T) {
	const want = "synthetic benchmark input"
	path := filepath.Join(t.TempDir(), "fixture.zip")
	if err := os.WriteFile(path, []byte(want), 0600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/input" {
			t.Error("unexpected source request")
		}
		_, _ = io.WriteString(w, want)
	}))
	t.Cleanup(server.Close)
	for _, source := range [][2]string{{path, ""}, {"", server.URL + "/input"}} {
		reader, err := openBenchmarkSource(context.Background(), source[0], source[1])
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil || string(data) != want {
			t.Fatalf("source bytes changed: error=%v", err)
		}
	}
}

func TestBenchmarkSourceRejectsAmbiguousAndNonLoopbackURLs(t *testing.T) {
	for _, source := range [][2]string{
		{"", ""}, {"fixture.zip", "http://127.0.0.1:8766/input"},
		{"", "https://127.0.0.1:8766/input"}, {"", "http://localhost:8766/input"},
		{"", "http://10.0.2.2:8766/input"}, {"", "http://example.invalid:8766/input"},
		{"", "http://127.0.0.1/input"}, {"", "http://127.0.0.1:0/input"},
		{"", "http://127.0.0.1:65536/input"}, {"", "http://user:secret@127.0.0.1:8766/input"},
		{"", "http://127.0.0.1:8766/input#fragment"}, {"", "file:///input.zip"},
	} {
		if reader, err := openBenchmarkSource(context.Background(), source[0], source[1]); err == nil {
			_ = reader.Close()
			t.Fatal("invalid source was accepted")
		}
	}
}

func TestBenchmarkSourceRejectsRedirectsFailuresAndCancellation(t *testing.T) {
	var redirected atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, "/destination", http.StatusFound)
		case "/destination":
			redirected.Store(true)
		default:
			http.Error(w, "synthetic failure", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	for _, endpoint := range []string{"/redirect", "/missing"} {
		if reader, err := openBenchmarkSource(context.Background(), "", server.URL+endpoint); err == nil {
			_ = reader.Close()
			t.Fatal("unavailable source was accepted")
		}
	}
	if redirected.Load() {
		t.Fatal("source followed a redirect")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if reader, err := openBenchmarkSource(ctx, "", server.URL+"/destination"); err == nil {
		_ = reader.Close()
		t.Fatal("cancelled source was accepted")
	}
}

type rejectingBenchmarkTransport struct{ called *atomic.Bool }

func (transport rejectingBenchmarkTransport) RoundTrip(*http.Request) (*http.Response, error) {
	transport.called.Store(true)
	return nil, context.Canceled
}

func TestBenchmarkSourceDoesNotUseGlobalTransportOrProxy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "synthetic local input")
	}))
	t.Cleanup(server.Close)
	var called atomic.Bool
	previous := http.DefaultTransport
	http.DefaultTransport = rejectingBenchmarkTransport{&called}
	t.Cleanup(func() { http.DefaultTransport = previous })
	t.Setenv("HTTP_PROXY", "http://example.invalid:8766")
	reader, err := openBenchmarkSource(context.Background(), "", server.URL+"/input")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil || string(data) != "synthetic local input" || called.Load() {
		t.Fatalf("source did not remain direct and local: error=%v global=%v", err, called.Load())
	}
}
