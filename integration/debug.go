package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mirusu400/aram-core/application"
	aramcore "github.com/mirusu400/aram-core/core"
	"github.com/mirusu400/aram-frontend/frontend"
)

const (
	coreDebugSchemaVersion = 1
	coreDebugTraceEntries  = 4096
)

type coreDebugReport struct {
	SchemaVersion int                        `json:"schema_version"`
	Backend       string                     `json:"backend"`
	Diagnostics   Diagnostics                `json:"diagnostics"`
	Snapshot      *application.DebugSnapshot `json:"snapshot,omitempty"`
}

type coreDebugSnapshotter interface {
	DebugSnapshot(int) application.DebugSnapshot
}

func (backend *Backend) DebugArtifacts(
	ctx context.Context,
) ([]frontend.DebugArtifact, error) {
	if err := backend.lockOperation(ctx); err != nil {
		return nil, err
	}
	defer backend.operationMu.Unlock()

	diagnostics := backend.Diagnostics()
	report := coreDebugReport{
		SchemaVersion: coreDebugSchemaVersion,
		Backend:       backend.BackendName(),
		Diagnostics:   diagnostics,
	}
	machine := backend.currentMachine()
	if provider, ok := machine.(coreDebugSnapshotter); ok {
		snapshot := provider.DebugSnapshot(coreDebugTraceEntries)
		report.Snapshot = &snapshot
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode aram-core debug report: %w", err)
	}
	data = append(data, '\n')

	return []frontend.DebugArtifact{
		{
			Name:      "core.json",
			MediaType: "application/json",
			Data:      data,
		},
		{
			Name:      "core.log",
			MediaType: "text/plain; charset=utf-8",
			Data:      renderCoreDebugLog(report.Snapshot),
		},
	}, nil
}

func (backend *Backend) lockOperation(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if backend.operationMu.TryLock() {
		return nil
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if backend.operationMu.TryLock() {
				return nil
			}
		}
	}
}

func renderCoreDebugLog(snapshot *application.DebugSnapshot) []byte {
	var output strings.Builder
	if snapshot == nil {
		output.WriteString("[core]\nno application machine is loaded\n")
		return []byte(output.String())
	}
	fmt.Fprintf(
		&output,
		"[core]\nruntime=%s state=%s\n",
		snapshot.Runtime,
		snapshot.State,
	)
	writeDebugLogSection(&output, "guest-log", snapshot.GuestLog)
	writeDebugLogSection(&output, "host-trace", snapshot.HostTrace)
	return []byte(output.String())
}

func writeDebugLogSection(
	output *strings.Builder,
	name string,
	log application.DebugLogSnapshot,
) {
	fmt.Fprintf(
		output,
		"\n[%s] total=%d omitted=%d\n",
		name,
		log.Total,
		log.Omitted,
	)
	for _, entry := range log.Entries {
		output.WriteString(entry)
		output.WriteByte('\n')
	}
}

var (
	_ frontend.DebugExportBackend = (*Backend)(nil)
	_ aramcore.Machine            = (*application.Machine)(nil)
)
