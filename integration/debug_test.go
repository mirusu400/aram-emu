package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mirusu400/aram-core/application"
	"github.com/mirusu400/aram-frontend/frontend"
)

type stubMemoryProvider struct {
	regions []application.DebugMemoryRegion
}

func (s stubMemoryProvider) DebugMemoryRegions(int) []application.DebugMemoryRegion {
	return s.regions
}

func TestCoreMemoryArtifactsPackFaultedRegions(t *testing.T) {
	provider := stubMemoryProvider{regions: []application.DebugMemoryRegion{
		{Label: "pc", Base: 0x11000, Data: []byte{1, 2, 3, 4}},
		{Label: "stack", Base: 0x12000, Data: []byte{9, 8}},
		{Label: "empty", Base: 0x0, Data: nil},
	}}
	artifacts, err := coreMemoryArtifacts(provider)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 2 ||
		artifacts[0].Name != "core.mem.json" ||
		artifacts[1].Name != "core.mem.bin" {
		t.Fatalf("artifacts = %+v", artifacts)
	}
	if want := []byte{1, 2, 3, 4, 9, 8}; !bytes.Equal(artifacts[1].Data, want) {
		t.Fatalf("core.mem.bin = %v, want %v", artifacts[1].Data, want)
	}
	var index coreMemoryIndex
	if err := json.Unmarshal(artifacts[0].Data, &index); err != nil {
		t.Fatal(err)
	}
	if index.TotalBytes != 6 || len(index.Regions) != 2 {
		t.Fatalf("index = %+v", index)
	}
	if index.Regions[0] != (coreMemoryRegion{
		Label: "pc", Base: "0x00011000", Offset: 0, Size: 4,
	}) {
		t.Fatalf("region 0 = %+v", index.Regions[0])
	}
	if index.Regions[1] != (coreMemoryRegion{
		Label: "stack", Base: "0x00012000", Offset: 4, Size: 2,
	}) {
		t.Fatalf("region 1 = %+v", index.Regions[1])
	}
}

func TestCoreMemoryArtifactsAbsentWithoutRegions(t *testing.T) {
	if artifacts, err := coreMemoryArtifacts(stubMemoryProvider{}); err != nil ||
		artifacts != nil {
		t.Fatalf("empty-region artifacts = %+v err = %v", artifacts, err)
	}
	if artifacts, err := coreMemoryArtifacts(struct{}{}); err != nil ||
		artifacts != nil {
		t.Fatalf("non-provider artifacts = %+v err = %v", artifacts, err)
	}
}

func TestDebugArtifactsContainCoreSnapshotWithoutSourcePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic.dat")
	if err := os.WriteFile(path, syntheticEADS(), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := NewBackend(nil)
	t.Cleanup(func() { _ = backend.Close() })
	if _, err := backend.Open(
		context.Background(),
		frontend.OpenRequest{Path: path},
	); err != nil {
		t.Fatal(err)
	}
	if err := backend.Execute(
		context.Background(),
		frontend.CommandStart,
	); err != nil {
		t.Fatal(err)
	}

	artifacts, err := backend.DebugArtifacts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 2 ||
		artifacts[0].Name != "core.json" ||
		artifacts[1].Name != "core.log" {
		t.Fatalf("debug artifacts = %+v", artifacts)
	}
	var report coreDebugReport
	if err := json.Unmarshal(artifacts[0].Data, &report); err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != coreDebugSchemaVersion ||
		report.Backend != "aram-core" ||
		report.Diagnostics.Input.SHA256 == "" ||
		report.Snapshot == nil ||
		report.Snapshot.Runtime == "" ||
		report.Snapshot.CPU == nil ||
		report.Snapshot.LastResult == nil {
		t.Fatalf(
			"core debug report = %+v snapshot = %+v",
			report,
			report.Snapshot,
		)
	}
	if strings.Contains(string(artifacts[0].Data), path) ||
		strings.Contains(string(artifacts[1].Data), path) {
		t.Fatal("debug artifacts contain the host source path")
	}
	log := string(artifacts[1].Data)
	if !strings.Contains(log, "[core]") ||
		!strings.Contains(log, "[guest-log]") ||
		!strings.Contains(log, "[host-trace]") {
		t.Fatalf("core log = %q", log)
	}
}

func TestDebugArtifactsHonorCanceledContextWhileBackendIsBusy(t *testing.T) {
	backend := NewBackend(nil)
	backend.operationMu.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := backend.DebugArtifacts(ctx)
	backend.operationMu.Unlock()
	if err != context.Canceled {
		t.Fatalf("DebugArtifacts error = %v, want %v", err, context.Canceled)
	}
}
