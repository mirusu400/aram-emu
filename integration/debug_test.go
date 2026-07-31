package integration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mirusu400/aram-frontend/frontend"
)

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
