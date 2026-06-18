//go:build cli_tests && !windows

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
package cli_tests

import (
	"os/exec"
	"syscall"
)

// newProcessGroupAttr leaves the child in this test's process group. SIGINT is
// delivered to the parent pid directly, so a new group is not needed.
func newProcessGroupAttr() *syscall.SysProcAttr {
	return nil
}

// gracefulStop sends SIGINT to the parent, which the quickstart cluster parent
// handles as its stop signal and relays to its children.
func gracefulStop(cmd *exec.Cmd) error {
	return cmd.Process.Signal(syscall.SIGINT)
}

// forceKillTree kills the parent process.
func forceKillTree(cmd *exec.Cmd) {
	_ = cmd.Process.Kill()
}
