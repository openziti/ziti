//go:build windows

package cmd

import "os"

func getFileMode(isPrivateKey bool) os.FileMode {
	// Default modes for Windows:
	// - Private keys: 0600 (rw-------)
	// - Public files: 0644 (rw-r--r--)
	if isPrivateKey {
		return os.FileMode(0600)
	}
	return os.FileMode(0644)
}
