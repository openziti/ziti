//go:build cli_tests && windows

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
	"strconv"
	"syscall"

	"golang.org/x/sys/windows"
)

// newProcessGroupAttr starts the child in its own process group so it can be
// targeted by a CTRL_BREAK console event while still sharing this test's console.
func newProcessGroupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
}

// gracefulStop delivers CTRL_BREAK to the child's process group. The Go runtime
// maps it to SIGINT, which the quickstart cluster parent handles as its stop
// signal. It fails if this process has no console attached.
func gracefulStop(cmd *exec.Cmd) error {
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(cmd.Process.Pid))
}

// forceKillTree kills the child and its descendants.
func forceKillTree(cmd *exec.Cmd) {
	_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
}
