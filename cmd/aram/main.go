//go:build (windows || linux || darwin) && !android && !ios

package main

import (
	"context"
	"fmt"
	"os"
	"strings"

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
	relaunchArgs := updateRelaunchArguments(backend.relaunchArgs, update.RelaunchPath)
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
	arguments, err := parseDesktopArguments(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "aram:", err)
		fmt.Fprintln(os.Stderr, "usage: aram [--profile <profile-id>] <local-input>")
		os.Exit(2)
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

	initialArgument := arguments.initialArgument
	initialPath := initialArgument
	initialLink := ""
	if _, recognized, _ := remoteinput.Parse(initialPath); recognized {
		initialLink = initialPath
		initialPath = ""
	}
	if arguments.profileID != "" {
		// Queue the fully described initial request instead of opening its path
		// twice. Later File/Open, drops and links retain their normal behavior.
		initialPath = ""
	}
	backend := &productBackend{
		Backend: hostbackend.NewBackend(hostbackend.Options{
			System: hostbackend.DefaultSystemOptions(),
		}),
		relaunchArgs: arguments.relaunchArguments(),
	}
	defer backend.Close()
	if err := frontend.RunWithDesktopShell(backend, initialPath, func(shell *frontend.Shell) {
		if arguments.profileID != "" {
			shell.OpenExternalRequest(arguments.localOpenRequest())
		}
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

type desktopArguments struct {
	initialArgument string
	profileID       string
	openOnStart     bool
}

func parseDesktopArguments(args []string) (desktopArguments, error) {
	var result desktopArguments
	var positional []string
	profileSeen := false
	for i := 0; i < len(args); i++ {
		argument := args[i]
		if argument == "--profile" || strings.HasPrefix(argument, "--profile=") {
			if profileSeen {
				return result, fmt.Errorf("--profile may only be specified once")
			}
			profileSeen = true
			value := strings.TrimPrefix(argument, "--profile=")
			if argument == "--profile" {
				i++
				if i >= len(args) {
					return result, fmt.Errorf("--profile requires a profile ID")
				}
				value = args[i]
			}
			if strings.TrimSpace(value) == "" || strings.HasPrefix(value, "-") {
				return result, fmt.Errorf("--profile requires a non-empty profile ID")
			}
			result.profileID = value
			continue
		}
		positional = append(positional, argument)
	}
	result.initialArgument, result.openOnStart = parseArguments(positional)
	if profileSeen {
		count := 0
		for _, argument := range positional {
			if argument != openAfterInstallArgument {
				count++
			}
		}
		if count != 1 || strings.TrimSpace(result.initialArgument) == "" {
			return result, fmt.Errorf("--profile requires exactly one local input")
		}
		if _, recognized, _ := remoteinput.Parse(result.initialArgument); recognized {
			return result, fmt.Errorf("--profile cannot be used with a remote link; open a local input instead")
		}
	}
	return result, nil
}

func (args desktopArguments) localOpenRequest() frontend.OpenRequest {
	return frontend.OpenRequest{Path: args.initialArgument, ProfileID: args.profileID}
}

func (args desktopArguments) relaunchArguments() []string {
	if args.profileID != "" {
		return []string{"--profile", args.profileID, args.initialArgument}
	}
	if args.initialArgument != "" {
		return []string{args.initialArgument}
	}
	return []string{openAfterInstallArgument}
}

func updateRelaunchArguments(initial []string, path string) []string {
	if path == "" {
		return initial
	}
	args, err := parseDesktopArguments(initial)
	if err == nil && args.initialArgument == path {
		return initial
	}
	// An update reopening a different input must not inherit the initial override.
	return []string{path}
}
