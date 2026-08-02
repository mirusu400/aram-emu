//go:build !windows

package bootstrap

import (
	"os"
	"os/exec"
)

func configureLaunchCommand(command *exec.Cmd) {
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
}
