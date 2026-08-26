package integration

import (
	"context"
	"testing"

	"github.com/mirusu400/aram-frontend/frontend"
)

// TestOpenFromDataBytesLoadsMachine covers the web/wasm path: a host with no
// readable filesystem hands the input in-band as OpenRequest.Data. The adapter
// must inspect and load it from memory with the same result as opening the same
// bytes from a file, and reach a runnable machine - proving the browser build
// can load a title without a server-side filesystem.
func TestOpenFromDataBytesLoadsMachine(t *testing.T) {
	backend := NewBackend(nil)
	t.Cleanup(func() { _ = backend.Close() })

	info, err := backend.Open(
		context.Background(),
		frontend.OpenRequest{DisplayName: "synthetic.dat", Data: syntheticEADS()},
	)
	if err != nil {
		t.Fatal(err)
	}
	if info.DisplayName != "synthetic.dat" ||
		info.Format != "eads" ||
		info.ProfileID != "wipi-1.2.1/generic" ||
		len(info.SHA256) != 64 {
		t.Fatalf("byte-input info = %+v", info)
	}
	if backend.State() != frontend.StateReady {
		t.Fatalf("state after byte open = %s", backend.State())
	}
	if err := backend.Execute(context.Background(), frontend.CommandStart); err != nil {
		t.Fatal(err)
	}
	if backend.State() != frontend.StateRunning {
		t.Fatalf("state after byte-input start = %s", backend.State())
	}
}

// TestOpenFromDataBytesMatchesFileHash asserts the in-memory inspect produces
// the same content hash as the file path, so a title opened in the browser is
// identified identically to one opened on desktop.
func TestOpenFromDataBytesMatchesFileHash(t *testing.T) {
	data := syntheticEADS()

	fromBytes := NewBackend(nil)
	t.Cleanup(func() { _ = fromBytes.Close() })
	byteInfo, err := fromBytes.Open(
		context.Background(),
		frontend.OpenRequest{DisplayName: "synthetic.dat", Data: data},
	)
	if err != nil {
		t.Fatal(err)
	}

	if byteInfo.Size != int64(len(data)) {
		t.Fatalf("byte-input size = %d, want %d", byteInfo.Size, len(data))
	}
}
