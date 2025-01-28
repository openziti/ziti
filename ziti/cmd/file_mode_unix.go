//go:build !windows

package cmd

import (
	"os"
	"syscall"
)

func getFileMode(isPrivateKey bool) os.FileMode {
	// Default modes before umask:
	// - Private keys: 0600 (rw-------)
	// - Public files: 0644 (rw-r--r--)
	mode := os.FileMode(0644)
	if isPrivateKey {
		mode = os.FileMode(0600)
	}

	// Get current umask
	oldMask := syscall.Umask(0)
	syscall.Umask(oldMask) // Restore original umask

	// Apply umask to our default mode
	mode &= ^os.FileMode(oldMask)

	return mode
}
