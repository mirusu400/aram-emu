//go:build system_firmware && (windows || linux || darwin) && !android && !ios

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/mirusu400/aram-core/systemmachine"
	"github.com/mirusu400/aram-emu/systemintegration"
	"github.com/mirusu400/aram-frontend/frontend"
)

func main() {
	instructionsPerFrame := flag.Uint64(
		"instructions-per-frame",
		systemintegration.DefaultInstructionsPerFrame,
		"guest instructions executed by each frontend presentation quantum",
	)
	noMedia := flag.Bool(
		"no-media-persistence",
		false,
		"do not restore or save writable NAND media",
	)
	cpuBackend := flag.String(
		"cpu",
		string(systemmachine.CPUBackendJIT),
		"whole-phone CPU backend: jit or precise",
	)
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [options] [firmware-directory]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() > 1 {
		flag.Usage()
		os.Exit(2)
	}
	initialPath := ""
	if flag.NArg() == 1 {
		initialPath = flag.Arg(0)
	}
	backend := systemintegration.NewBackend(systemintegration.Options{
		InstructionsPerFrame:    *instructionsPerFrame,
		DisableMediaPersistence: *noMedia,
		CPUBackendMode:          systemmachine.CPUBackendMode(*cpuBackend),
	})
	defer backend.Close()
	if err := frontend.RunWithOptions(backend, initialPath, initialPath == ""); err != nil {
		fmt.Fprintln(os.Stderr, "aram-system:", err)
		os.Exit(1)
	}
}
