package integration

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mirusu400/aram-core/application"
	"github.com/mirusu400/aram-core/loader"
	"github.com/mirusu400/aram-frontend/frontend"
)

func TestGNEX32VirusOrdinaryProductPath(t *testing.T) {
	root := os.Getenv("ARAM_TEST_DATA")
	if root == "" {
		t.Skip("ARAM_TEST_DATA is not set")
	}
	var path string
	errFound := fmt.Errorf("Virus package found")
	err := filepath.WalkDir(root, func(candidate string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(candidate), ".zip") {
			return nil
		}
		data, err := os.ReadFile(candidate)
		if err != nil {
			return err
		}
		if fmt.Sprintf("%x", sha256.Sum256(data)) == application.GNEX32VirusSHA256 {
			path = candidate
			return errFound
		}
		return nil
	})
	if err != nil && err != errFound {
		t.Fatal(err)
	}
	if path == "" {
		t.Skip("Virus package not present in ARAM_TEST_DATA")
	}
	backend := NewBackend(nil)
	t.Cleanup(func() { _ = backend.Close() })
	info, err := backend.Open(context.Background(), frontend.OpenRequest{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if info.Format != string(loader.KindGNEX) || info.ProfileID != application.GNEX32VirusProfileID {
		t.Fatalf("product identity = %q, %q", info.Format, info.ProfileID)
	}
	if err := backend.Execute(context.Background(), frontend.CommandStart); err != nil {
		t.Fatal(err)
	}
	if before := backend.Diagnostics().GVM; before == nil || before.PresentCount != 0 || before.FrameValid {
		t.Fatalf("presentation before first Flush = %+v", before)
	}
	for frame := 0; frame < 9; frame++ {
		if err := backend.RunFrame(context.Background()); err != nil {
			t.Fatalf("frame %d: %v", frame, err)
		}
	}
	diagnostics := backend.Diagnostics()
	if diagnostics.GVM == nil || diagnostics.GVM.PresentCount != 1 || !diagnostics.GVM.FrameValid {
		t.Fatalf("GNEX32 presentation = %+v", diagnostics.GVM)
	}
}
