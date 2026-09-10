//go:build (windows || linux || darwin) && !android && !ios

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/mirusu400/aram-emu/hostbackend"
	"github.com/mirusu400/aram-emu/internal/bootstrap"
	"github.com/mirusu400/aram-emu/internal/remoteinput"
	"github.com/mirusu400/aram-frontend/frontend"
)

const openAfterInstallArgument = "--aram-open-after-install"

// keptPreviousRuntimes is how many superseded runtimes survive a prune. One
// covers the runtime that installed the update and is still shutting down.
const keptPreviousRuntimes = 1

type productBackend struct {
	*hostbackend.Backend
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
	// `aram save ...` is a headless save backup utility. It runs in this process
	// rather than launching the GUI or handing off to an installed runtime.
	if len(os.Args) >= 2 && os.Args[1] == "save" {
		os.Exit(runSaveCommand(os.Args[2:]))
	}

	if forwarded, err := bootstrap.ForwardToInstalled(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "aram: installed runtime:", err)
	} else if forwarded {
		return
	}

	// Installing an update leaves the runtime it replaced behind. Clearing
	// superseded runtimes at startup rather than at install time lets the
	// process that launched this one finish exiting first.
	if err := bootstrap.PruneRuntimes(keptPreviousRuntimes); err != nil {
		fmt.Fprintln(os.Stderr, "aram: superseded runtimes:", err)
	}
	if err := configurePlatformDeepLinks(); err != nil {
		fmt.Fprintln(os.Stderr, "aram: deep links:", err)
	}

	initialArgument, _ := parseArguments(os.Args[1:])
	initialPath := initialArgument
	initialLink := ""
	if _, recognized, _ := remoteinput.Parse(initialPath); recognized {
		initialLink = initialPath
		initialPath = ""
	}
	relaunchArgs := []string{openAfterInstallArgument}
	if initialArgument != "" {
		relaunchArgs = []string{initialArgument}
	}
	backend := &productBackend{
		Backend: hostbackend.NewBackend(hostbackend.Options{
			System: hostbackend.DefaultSystemOptions(),
		}),
		relaunchArgs: relaunchArgs,
	}
	defer backend.Close()
	if err := frontend.RunWithDesktopShell(backend, initialPath, func(shell *frontend.Shell) {
		links := make(chan string, 8)
		go func() {
			for link := range links {
				openRemoteLink(shell, link)
			}
		}()
		if initialLink != "" {
			links <- initialLink
		}
		go func() {
			for link := range platformDeepLinkEvents() {
				links <- link
			}
		}()
	}); err != nil {
		fmt.Fprintln(os.Stderr, "aram:", err)
		os.Exit(1)
	}
}

func openRemoteLink(shell *frontend.Shell, raw string) {
	shell.ReportExternalOpenStatus("Downloading and verifying linked package...")
	path, spec, err := remoteinput.Resolve(context.Background(), nil, "", raw)
	if err != nil {
		message := "Open link: " + err.Error()
		shell.ReportExternalOpenStatus(message)
		fmt.Fprintln(os.Stderr, "aram:", message)
		return
	}
	shell.OpenExternalRequest(frontend.OpenRequest{
		Path:           path,
		DisplayName:    spec.Name,
		ExpectedSHA256: spec.SHA256,
	})
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
