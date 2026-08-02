package bootstrap

import "os/exec"

func configureLaunchCommand(_ *exec.Cmd) {
	// Ebitengine's hideconsole package can call FreeConsole during package
	// initialization. os.Stdin, os.Stdout, and os.Stderr then retain Windows
	// handles which are no longer valid. Leaving the streams unset gives the
	// relaunched GUI process safe null handles instead of making CreateProcess
	// fail with ERROR_INVALID_HANDLE.
}
