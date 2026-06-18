//go:build windows

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

package run

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// configureChildProcAttr starts each child in a new process group, so it is
// addressable for GenerateConsoleCtrlEvent and the console's Ctrl-C does not
// reach it directly. The parent controls shutdown via relayStop.
func configureChildProcAttr(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NEW_PROCESS_GROUP
}

// relayStop sends CTRL_BREAK to the child's process group. The Go runtime maps
// CTRL_BREAK to SIGINT, which the quickstart node handles as its shutdown
// trigger. The child's pid is its process group id (CREATE_NEW_PROCESS_GROUP).
func relayStop(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(cmd.Process.Pid))
	}
}
