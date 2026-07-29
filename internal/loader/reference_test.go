package loader_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mirusu400/aram-emu/internal/loader/abhs"
	"github.com/mirusu400/aram-emu/internal/loader/eads"
)

func TestMagicholeReferenceDAT(t *testing.T) {
	reference := os.Getenv("ARAM_REFERENCE_REPO")
	if reference == "" {
		t.Skip("ARAM_REFERENCE_REPO is not set")
	}
	path := filepath.Join(
		reference,
		"SCH-W380_DL21",
		"SCH-W830_DL21_29360128_DL21.dat",
	)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("reference DAT is unavailable: %v", err)
	}

	modules := abhs.Inspect(data)
	if len(modules) != 6 {
		t.Fatalf("ABHS module count = %d, want 6", len(modules))
	}
	images := eads.Inspect(data)
	if len(images) != 1 {
		t.Fatalf("EADS image count = %d, want 1", len(images))
	}
	if images[0].Name != "MinigameQVGAOEM" {
		t.Fatalf("EADS image name = %q", images[0].Name)
	}
}
