//go:build (windows || linux || darwin) && !android && !ios

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mirusu400/aram-emu/hostbackend"
	"github.com/mirusu400/aram-frontend/frontend"
)

// runSaveCommand implements the headless `aram save` utility: it backs up and
// restores a title's writable storage as a portable file, so a save can be
// copied off the machine and survive loss of the local state directory. It runs
// in this process without launching the GUI or forwarding to an installed
// runtime, and returns a process exit code.
func runSaveCommand(args []string) int {
	if len(args) == 0 {
		return saveUsage()
	}
	switch args[0] {
	case "export":
		return runSaveExport(args[1:])
	case "import":
		return runSaveImport(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "aram save: unknown subcommand %q\n", args[0])
		return saveUsage()
	}
}

func saveUsage() int {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  aram save export <input> [backup.aramsave]")
	fmt.Fprintln(os.Stderr, "  aram save import <input> <backup.aramsave>")
	fmt.Fprintln(os.Stderr,
		"\nBacks up or restores the save (writable storage) of a title, keyed to")
	fmt.Fprintln(os.Stderr,
		"the exact input. Restore refuses a backup made for a different title.")
	return 2
}

// openSaveBackend opens the input headlessly against the same default state
// directory the product uses, so a CLI export reads and a CLI import writes the
// very save file the GUI keeps.
func openSaveBackend(input string) (*hostbackend.Backend, frontend.InputInfo, error) {
	backend := hostbackend.NewBackend(hostbackend.Options{
		System: hostbackend.DefaultSystemOptions(),
	})
	info, err := backend.Open(context.Background(), frontend.OpenRequest{Path: input})
	if err != nil {
		_ = backend.Close()
		return nil, frontend.InputInfo{}, err
	}
	return backend, info, nil
}

func runSaveExport(args []string) int {
	if len(args) < 1 || len(args) > 2 {
		return saveUsage()
	}
	input := args[0]
	backend, info, err := openSaveBackend(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "aram save export: open %s: %v\n", input, err)
		return 1
	}
	defer backend.Close()

	blob, err := backend.ExportSaveData()
	if err != nil {
		fmt.Fprintf(os.Stderr, "aram save export: %v\n", err)
		return 1
	}

	dst := ""
	if len(args) == 2 {
		dst = args[1]
	} else {
		dst = defaultBackupName(input, info.SHA256)
	}
	if err := os.WriteFile(dst, blob, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "aram save export: write %s: %v\n", dst, err)
		return 1
	}
	fmt.Printf("saved %d bytes to %s\n", len(blob), dst)
	return 0
}

func runSaveImport(args []string) int {
	if len(args) != 2 {
		return saveUsage()
	}
	input, src := args[0], args[1]
	blob, err := os.ReadFile(src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "aram save import: read %s: %v\n", src, err)
		return 1
	}
	backend, _, err := openSaveBackend(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "aram save import: open %s: %v\n", input, err)
		return 1
	}
	defer backend.Close()

	if err := backend.ImportSaveData(blob); err != nil {
		fmt.Fprintf(os.Stderr, "aram save import: %v\n", err)
		return 1
	}
	fmt.Printf("restored %s into %s\n", src, input)
	return 0
}

// defaultBackupName derives a friendly, filesystem-safe backup name from the
// input file and a short slice of its identity, so two saves for different
// titles never collide.
func defaultBackupName(input, hash string) string {
	base := filepath.Base(input)
	if ext := filepath.Ext(base); ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	base = sanitizeBackupName(base)
	if base == "" {
		base = "aram"
	}
	short := hash
	if len(short) > 8 {
		short = short[:8]
	}
	if short != "" {
		base = base + "-" + short
	}
	return base + ".aramsave"
}

func sanitizeBackupName(name string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '-', r == '_', r == '.':
			return r
		default:
			return '_'
		}
	}, name)
}
