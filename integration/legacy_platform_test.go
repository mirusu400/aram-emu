package integration

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"testing"

	"github.com/mirusu400/aram-core/application"
	"github.com/mirusu400/aram-frontend/frontend"
)

func TestLegacyRecognitionErrorsPreserveInputIdentity(t *testing.T) {
	mif := make([]byte, 64)
	for offset, value := range map[int]uint32{0: 0x10011, 8: 32, 12: 8, 16: 40, 20: 1, 24: 48, 28: 16} {
		binary.LittleEndian.PutUint32(mif[offset:], value)
	}
	brew := syntheticZIP(t, map[string][]byte{"app.mif": mif, "app.mod": []byte("opaque synthetic module")})
	// Synthetic header and undecoded body. These bytes claim no GVM instruction
	// semantics, matching the core's deliberately recognition-only contract.
	sgs := append([]byte{1, 0xff, 0x0c, 0xff, 0xff, 1, 0, 0, 0, 0}, []byte("Synthetic\x00\x00\x01\x02\x03\x04")...)
	for _, test := range []struct {
		name, format, profile string
		data                  []byte
	}{
		{"synthetic.zip", "brew-package", "brew-container-v1/unknown/generic", brew},
		{"synthetic.sgs", "gnex-sgs", "gvm-container-v1/skt/generic", sgs},
	} {
		t.Run(test.format, func(t *testing.T) {
			backend := NewBackend(nil)
			t.Cleanup(func() { _ = backend.Close() })
			info, err := backend.Open(context.Background(), frontend.OpenRequest{DisplayName: test.name, Data: test.data})
			var backendErr *frontend.BackendError
			var unsupported *application.UnsupportedPlatformError
			if !errors.As(err, &backendErr) || backendErr.Kind != frontend.FailureUnsupportedProfile ||
				!errors.As(err, &unsupported) || !errors.Is(err, application.ErrUnsupportedSource) {
				t.Fatalf("recognition must preserve typed unsupported execution: %v", err)
			}
			if info.Format != test.format || info.ProfileID != test.profile || info.DisplayName != test.name ||
				info.Size != int64(len(test.data)) || info.SHA256 != fmt.Sprintf("%x", sha256.Sum256(test.data)) {
				t.Fatalf("recognized identity = %+v", info)
			}
			if backend.State() != frontend.StateEmpty || backend.Supports(frontend.CommandStart) {
				t.Fatal("recognition-only package became executable")
			}
		})
	}
}

func TestJ2MESaveStateRejectsAnotherResolvedProfile(t *testing.T) {
	ctx := context.Background()
	backend := NewBackend(nil)
	t.Cleanup(func() { _ = backend.Close() })
	if err := backend.ConfigureStateRoot(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	request := frontend.OpenRequest{DisplayName: "synthetic.jar", Data: syntheticJ2ME(t, false)}
	if _, err := backend.Open(ctx, request); err != nil {
		t.Fatal(err)
	}
	for _, command := range []frontend.BackendCommand{frontend.CommandStart, frontend.CommandSaveState} {
		if err := backend.Execute(ctx, command); err != nil {
			t.Fatal(err)
		}
	}
	request.ProfileID = "j2me-1.0/lgt/generic"
	if _, err := backend.Open(ctx, request); err != nil {
		t.Fatal(err)
	}
	if err := backend.Execute(ctx, frontend.CommandLoadState); err == nil {
		t.Fatal("generic Java state was accepted under an LGT profile")
	}
	if backend.State() != frontend.StateReady || backend.Diagnostics().Input.ProfileID != request.ProfileID {
		t.Fatal("rejected state mutated the current Java profile or lifecycle")
	}
}
