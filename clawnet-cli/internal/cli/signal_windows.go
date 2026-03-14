//go:build windows

package cli

import "os"

func notifyResize(ch chan os.Signal) {
	// Windows does not support SIGWINCH; resize detection is a no-op.
}
