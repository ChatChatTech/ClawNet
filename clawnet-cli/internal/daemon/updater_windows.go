//go:build windows

package daemon

import (
	"os"
	"os/exec"
)

// restartSelf on Windows spawns a new process and exits the current one.
// Windows doesn't support exec(2) — use cmd /c approach instead.
func restartSelf(exePath string) {
	cmd := exec.Command(exePath, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Start()
	os.Exit(0)
}
