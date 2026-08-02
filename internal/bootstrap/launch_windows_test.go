package bootstrap

import (
	"os"
	"os/exec"
	"testing"
)

func TestConfigureLaunchCommandAvoidsDetachedConsoleHandles(t *testing.T) {
	command := exec.Command("unused")
	configureLaunchCommand(command)

	if command.Stdin != nil || command.Stdout != nil || command.Stderr != nil {
		t.Fatal("Windows launch inherited potentially detached console handles")
	}
}

func TestLaunchIgnoresClosedStandardStreams(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	closed, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	if err := closed.Close(); err != nil {
		t.Fatal(err)
	}

	stdin, stdout, stderr := os.Stdin, os.Stdout, os.Stderr
	os.Stdin, os.Stdout, os.Stderr = closed, closed, closed
	defer func() {
		os.Stdin, os.Stdout, os.Stderr = stdin, stdout, stderr
	}()

	if err := Launch(executable, []string{
		"-test.run=^TestLaunchHelperProcess$",
		"-test.count=1",
	}); err != nil {
		t.Fatalf("Launch with detached standard streams: %v", err)
	}
}

func TestLaunchHelperProcess(*testing.T) {}
