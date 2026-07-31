//go:build (windows || linux || darwin) && !android && !ios

package main

import (
	"fmt"
	"os"

	"github.com/mirusu400/aram-emu/integration"
	"github.com/mirusu400/aram-emu/internal/bootstrap"
	"github.com/mirusu400/aram-frontend/frontend"
)

const openAfterInstallArgument = "--aram-open-after-install"

type productBackend struct {
	*integration.Backend
	relaunchArgs []string
}

func (backend *productBackend) InstallProductUpdate(
	update frontend.ProductUpdate,
) error {
	executable, err := bootstrap.Install(update.ArchivePath)
	if err != nil {
		return fmt.Errorf("install %s product update: %w", update.Channel, err)
	}
	relaunchArgs := backend.relaunchArgs
	if update.RelaunchPath != "" {
		relaunchArgs = []string{update.RelaunchPath}
	}
	if err := bootstrap.Launch(executable, relaunchArgs); err != nil {
		return err
	}
	return nil
}

func main() {
	if forwarded, err := bootstrap.ForwardToInstalled(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "aram: installed runtime:", err)
	} else if forwarded {
		return
	}

	initialPath, openOnStart := parseArguments(os.Args[1:])
	relaunchArgs := []string{openAfterInstallArgument}
	if initialPath != "" {
		relaunchArgs = []string{initialPath}
	}
	backend := &productBackend{
		Backend:      integration.NewBackend(nil),
		relaunchArgs: relaunchArgs,
	}
	defer backend.Close()
	if err := frontend.RunWithOptions(backend, initialPath, openOnStart); err != nil {
		fmt.Fprintln(os.Stderr, "aram:", err)
		os.Exit(1)
	}
}

func parseArguments(args []string) (string, bool) {
	initialPath := ""
	openOnStart := false
	for _, argument := range args {
		if argument == openAfterInstallArgument {
			openOnStart = true
			continue
		}
		if initialPath == "" {
			initialPath = argument
		}
	}
	return initialPath, openOnStart
}
