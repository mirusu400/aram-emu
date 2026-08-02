//go:build !windows

package bootstrap

import (
	"os"
	"os/exec"
	"testing"
)

func TestConfigureLaunchCommandInheritsStandardStreams(t *testing.T) {
	command := exec.Command("unused")
	configureLaunchCommand(command)

	if command.Stdin != os.Stdin ||
		command.Stdout != os.Stdout ||
		command.Stderr != os.Stderr {
		t.Fatal("non-Windows launch did not inherit the standard streams")
	}
}
