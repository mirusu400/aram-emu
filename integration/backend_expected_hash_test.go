package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mirusu400/aram-frontend/frontend"
)

func TestOpenEnforcesExpectedSHA256(t *testing.T) {
	data := syntheticEADS()
	path := filepath.Join(t.TempDir(), "synthetic.dat")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	backend := NewBackend(nil)
	t.Cleanup(func() { _ = backend.Close() })
	_, err := backend.Open(context.Background(), frontend.OpenRequest{
		Path:           path,
		ExpectedSHA256: strings.Repeat("0", 64),
	})
	if err == nil || !strings.Contains(err.Error(), "SHA-256 mismatch") {
		t.Fatalf("mismatched open error = %v", err)
	}

	digestBytes := sha256.Sum256(data)
	digest := hex.EncodeToString(digestBytes[:])
	info, err := backend.Open(context.Background(), frontend.OpenRequest{
		Path:           path,
		ExpectedSHA256: digest,
	})
	if err != nil {
		t.Fatal(err)
	}
	if info.SHA256 != digest {
		t.Fatalf("opened SHA-256 = %q, want %q", info.SHA256, digest)
	}
}

func TestOpenDataEnforcesExpectedSHA256(t *testing.T) {
	data := syntheticEADS()
	backend := NewBackend(nil)
	t.Cleanup(func() { _ = backend.Close() })

	_, err := backend.Open(context.Background(), frontend.OpenRequest{
		DisplayName:    "synthetic.dat",
		Data:           data,
		ExpectedSHA256: strings.Repeat("0", 64),
	})
	if err == nil || !strings.Contains(err.Error(), "SHA-256 mismatch") {
		t.Fatalf("mismatched byte open error = %v", err)
	}

	digestBytes := sha256.Sum256(data)
	digest := hex.EncodeToString(digestBytes[:])
	info, err := backend.Open(context.Background(), frontend.OpenRequest{
		DisplayName:    "synthetic.dat",
		Data:           data,
		ExpectedSHA256: digest,
	})
	if err != nil {
		t.Fatal(err)
	}
	if info.SHA256 != digest {
		t.Fatalf("opened byte SHA-256 = %q, want %q", info.SHA256, digest)
	}
}
