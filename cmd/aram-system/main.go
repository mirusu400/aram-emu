//go:build (windows || linux || darwin) && !android && !ios

// Command aram-system is the whole-phone development entry point. The
// shipping product (cmd/aram) runs firmware through the same shell; this
// exists for the flags that only matter while working on the system
// machine -- picking the instruction-precise core, choosing the frame
// quantum, and running without touching the writable NAND.

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/mirusu400/aram-core/application"
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
		application.FastestBackend,
		"whole-phone CPU backend: fastest, native, jit, or precise",
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
		CPUBackend:              *cpuBackend,
	})
	defer backend.Close()
	if err := frontend.RunWithOptions(backend, initialPath, initialPath == ""); err != nil {
		fmt.Fprintln(os.Stderr, "aram-system:", err)
		os.Exit(1)
	}
}
