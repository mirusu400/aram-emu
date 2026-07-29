package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mirusu400/aram-emu/internal/frontend"
	"github.com/mirusu400/aram-emu/internal/loader"
)

const version = "0.1.0-dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "aram:", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	if len(arguments) == 0 {
		return frontend.Run("")
	}
	switch arguments[0] {
	case "gui":
		initial := ""
		if len(arguments) > 1 {
			initial = arguments[1]
		}
		return frontend.Run(initial)
	case "inspect":
		return inspect(arguments[1:])
	case "version", "--version", "-version":
		fmt.Println("ARAM", version)
		return nil
	case "help", "--help", "-h":
		printUsage()
		return nil
	}
	if info, err := os.Stat(arguments[0]); err == nil && info.Mode().IsRegular() {
		return frontend.Run(arguments[0])
	}
	return fmt.Errorf("unknown command %q", arguments[0])
}

func inspect(arguments []string) error {
	flags := flag.NewFlagSet("inspect", flag.ContinueOnError)
	asJSON := flags.Bool("json", false, "print machine-readable JSON")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("usage: aram inspect [-json] <file>")
	}
	report, err := loader.InspectFile(flags.Arg(0))
	if err != nil {
		return err
	}
	if *asJSON {
		data, err := report.JSON()
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}
	fmt.Println("Path:   ", report.Path)
	fmt.Println("Format: ", report.Kind)
	fmt.Println("Size:   ", report.Size)
	fmt.Println("SHA-256:", report.SHA256)
	for _, marker := range report.Markers {
		fmt.Printf("Marker:  %s at 0x%x\n", marker.Magic, marker.Offset)
	}
	return nil
}

func printUsage() {
	name := filepath.Base(os.Args[0])
	fmt.Printf(`ARAM — Archived Runtime for ARM Mobiles

Usage:
  %[1]s                     Open the desktop emulator shell
  %[1]s gui [file]          Open the shell and optionally inspect a file
  %[1]s inspect [-json] f   Inspect a package or firmware image
  %[1]s version             Print the version
`, name)
}
