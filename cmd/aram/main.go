//go:build (windows || linux || darwin) && !android && !ios

package main

import (
	"fmt"
	"os"

	"github.com/mirusu400/aram-emu/integration"
	"github.com/mirusu400/aram-frontend/frontend"
)

func main() {
	initialPath := ""
	if len(os.Args) > 1 {
		initialPath = os.Args[1]
	}
	backend := integration.NewBackend(nil)
	defer backend.Close()
	if err := frontend.Run(backend, initialPath); err != nil {
		fmt.Fprintln(os.Stderr, "aram:", err)
		os.Exit(1)
	}
}
