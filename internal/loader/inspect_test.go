package loader

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInspectDATWithMarkers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.dat")
	data := make([]byte, 512)
	copy(data[128:], "ABHS")
	copy(data[400:], "EADS")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := InspectFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if report.Kind != KindDAT {
		t.Fatalf("Kind = %q, want %q", report.Kind, KindDAT)
	}
	if len(report.Markers) != 2 {
		t.Fatalf("len(Markers) = %d, want 2", len(report.Markers))
	}
	if report.Markers[0].Magic != "ABHS" || report.Markers[0].Offset != 128 {
		t.Fatalf("first marker = %#v", report.Markers[0])
	}
	if report.SHA256 == "" {
		t.Fatal("SHA256 is empty")
	}
}

func TestInspectRejectsDirectory(t *testing.T) {
	if _, err := InspectFile(t.TempDir()); err == nil {
		t.Fatal("InspectFile(directory) succeeded")
	}
}
