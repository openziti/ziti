//go:build !linux && !darwin && !freebsd && !windows

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

package zdecli

import (
	"fmt"
	"runtime"

	"github.com/pkg/errors"
)

// CommandName allow usage and such to be altered to fit a hosting executable
var CommandName = "ziti-db-explorer"

// Run always returns an error, since the interactive shell's prompt library does not build on this platform.
func Run(string, string) error {
	return errors.Errorf("the interactive database explorer is not supported on %s", runtime.GOOS)
}

func PrintUsage() {
	fmt.Printf("'%s' is not supported on %s\n", CommandName, runtime.GOOS)
}
