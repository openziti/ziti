//go:build !windows

/*
	Copyright NetFoundry Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/


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
