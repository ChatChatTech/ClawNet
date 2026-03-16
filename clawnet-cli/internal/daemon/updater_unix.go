//go:build !windows

package daemon

import (
	"os"
	"syscall"
)

// restartSelf replaces the running process with a new instance of the binary.
func restartSelf(exePath string) {
	syscall.Exec(exePath, os.Args, os.Environ())
}
