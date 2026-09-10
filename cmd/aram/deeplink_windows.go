//go:build windows && !android && !ios

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

func configurePlatformDeepLinks() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return err
	}
	root, _, err := registry.CreateKey(
		registry.CURRENT_USER,
		`Software\Classes\aram`,
		registry.SET_VALUE|registry.CREATE_SUB_KEY,
	)
	if err != nil {
		return fmt.Errorf("register aram protocol: %w", err)
	}
	defer root.Close()
	if err := root.SetStringValue("", "URL:ARAM Link"); err != nil {
		return err
	}
	if err := root.SetStringValue("URL Protocol", ""); err != nil {
		return err
	}
	command, _, err := registry.CreateKey(root, `shell\open\command`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer command.Close()
	return command.SetStringValue("", `"`+executable+`" "%1"`)
}
