// Command aram-iconpack derives packaging resources from the canonical ARAM
// frontend icon.
package main

import (
	"flag"
	"fmt"
	"image/png"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/mirusu400/aram-emu/internal/appicon"
)

func main() {
	sourcePath := flag.String("source", "", "canonical frontend icon PNG")
	icoPath := flag.String("ico", "", "output Windows ICO path")
	icnsPath := flag.String("icns", "", "output macOS ICNS path")
	androidRoot := flag.String("android-res", "", "output Android res directory")
	flag.Parse()

	if *sourcePath == "" {
		log.Fatal("-source is required")
	}
	if *icoPath == "" && *icnsPath == "" && *androidRoot == "" {
		log.Fatal("at least one output flag is required")
	}

	file, err := os.Open(*sourcePath)
	if err != nil {
		log.Fatalf("open source icon: %v", err)
	}
	source, err := png.Decode(file)
	closeErr := file.Close()
	if err != nil {
		log.Fatalf("decode source icon: %v", err)
	}
	if closeErr != nil {
		log.Fatalf("close source icon: %v", closeErr)
	}

	if *icoPath != "" {
		if err := writeOutput(*icoPath, func(writer io.Writer) error {
			return appicon.WriteICO(writer, source)
		}); err != nil {
			log.Fatal(err)
		}
	}
	if *icnsPath != "" {
		if err := writeOutput(*icnsPath, func(writer io.Writer) error {
			return appicon.WriteICNS(writer, source)
		}); err != nil {
			log.Fatal(err)
		}
	}
	if *androidRoot != "" {
		if err := appicon.WriteAndroidResources(*androidRoot, source); err != nil {
			log.Fatalf("write Android icons: %v", err)
		}
	}
}

func writeOutput(path string, write func(io.Writer) error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create output directory for %s: %w", path, err)
	}
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	if err := write(file); err != nil {
		file.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	return nil
}
