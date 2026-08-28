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
	// coreDebugMemoryLimit caps the total guest bytes packed into core.mem.bin.
	// The frontend applies its own per-artifact and bundle ceilings on top; this
	// keeps the request bounded before it ever reaches them.
	coreDebugMemoryLimit = 1 << 20
)

type coreDebugMemoryProvider interface {
	DebugMemoryRegions(int) []application.DebugMemoryRegion
}

// coreMemoryIndex describes the layout of core.mem.bin so a reader can slice
// each captured window back out without guessing boundaries.
type coreMemoryIndex struct {
	SchemaVersion int                `json:"schema_version"`
	TotalBytes    int                `json:"total_bytes"`
	Regions       []coreMemoryRegion `json:"regions"`
}

type coreMemoryRegion struct {
	Label  string `json:"label"`
	Base   string `json:"base"`
	Offset int    `json:"offset"`
	Size   int    `json:"size"`
}

type coreDebugReport struct {
	SchemaVersion int                        `json:"schema_version"`
	Backend       string                     `json:"backend"`
	Diagnostics   Diagnostics                `json:"diagnostics"`
	Snapshot      *application.DebugSnapshot `json:"snapshot,omitempty"`
}

type coreDebugSnapshotter interface {
	DebugSnapshot(int) application.DebugSnapshot
}

// CoreDebugSnapshot returns the compact machine counters used by compatibility
// and performance tooling without serializing a full debug artifact. The
// operation lock keeps it ordered with frame execution.
func (backend *Backend) CoreDebugSnapshot(
	maxEntries int,
) (application.DebugSnapshot, bool) {
	backend.operationMu.Lock()
	defer backend.operationMu.Unlock()
	machine := unwrapMachine(backend.currentMachine())
	provider, ok := machine.(coreDebugSnapshotter)
	if !ok {
		return application.DebugSnapshot{}, false
	}
	return provider.DebugSnapshot(maxEntries), true
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
	machine := unwrapMachine(backend.currentMachine())
	if provider, ok := machine.(coreDebugSnapshotter); ok {
		snapshot := provider.DebugSnapshot(coreDebugTraceEntries)
		report.Snapshot = &snapshot
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode aram-core debug report: %w", err)
	}
	data = append(data, '\n')

	artifacts := []frontend.DebugArtifact{
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
	}
	memoryArtifacts, err := coreMemoryArtifacts(machine)
	if err != nil {
		return nil, err
	}
	return append(artifacts, memoryArtifacts...), nil
}

// coreMemoryArtifacts packs the faulted machine's guest-memory windows into a
// raw core.mem.bin plus a core.mem.json index. It returns nothing unless the
// machine has faulted and exposed readable bytes, so a healthy session never
// carries guest memory in its bundle.
func coreMemoryArtifacts(machine any) ([]frontend.DebugArtifact, error) {
	provider, ok := machine.(coreDebugMemoryProvider)
	if !ok {
		return nil, nil
	}
	regions := provider.DebugMemoryRegions(coreDebugMemoryLimit)
	if len(regions) == 0 {
		return nil, nil
	}

	index := coreMemoryIndex{SchemaVersion: coreDebugSchemaVersion}
	var blob []byte
	for _, region := range regions {
		if len(region.Data) == 0 {
			continue
		}
		index.Regions = append(index.Regions, coreMemoryRegion{
			Label:  region.Label,
			Base:   fmt.Sprintf("0x%08x", region.Base),
			Offset: len(blob),
			Size:   len(region.Data),
		})
		blob = append(blob, region.Data...)
	}
	if len(blob) == 0 {
		return nil, nil
	}
	index.TotalBytes = len(blob)

	indexData, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode aram-core memory index: %w", err)
	}
	indexData = append(indexData, '\n')

	return []frontend.DebugArtifact{
		{
			Name:      "core.mem.json",
			MediaType: "application/json",
			Data:      indexData,
		},
		{
			Name:      "core.mem.bin",
			MediaType: "application/octet-stream",
			Data:      blob,
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
