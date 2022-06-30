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

package vagrantutil

import (
	"errors"
	"os"
	"strings"
)

// List of errors ignored by Destroy / Status methods:
const (
	errNotCreated    = "A Vagrant environment or target machine is required to run this"
	errRubyNotExists = "No such file or directory - getcwd (Errno::ENOENT)"
)

func isNotCreated(err error) bool {
	return os.IsNotExist(err) || strings.Contains(err.Error(), errNotCreated) ||
		strings.Contains(err.Error(), errRubyNotExists)
}

// The following errors are returned by the Wait function:
var (
	ErrBoxAlreadyExists  = errors.New("box already exists")
	ErrBoxInvalidVersion = errors.New("invalid box version")
	ErrBoxNotAvailable   = errors.New("box is not available")
	ErrBoxDownload       = errors.New("unable to download the box")
	ErrVirtualBox        = errors.New("VirtualBox is missing or not operational")
)

var defaultWaiter Waiter

// Waiter is used to consume output channel from a Vagrant command, parsing
// each line looking for an error when command execution fails.
type Waiter struct {
	// OutputFunc, when non-nil, is called each time a Wait method receives
	// a line of output from command channel.
	OutputFunc func(string)
}

// Wait is a convenience method that consumes Vagrant output looking
// for well-known errors.
//
// It returns an error variable for any known error condition, which makes it
// easier for the caller to recover from failure.
//
// If OutputFunc is non-nil, it's called on each line of output received
// from the out channel.
func (w *Waiter) Wait(out <-chan *CommandOutput, err error) error {
	var e error
	for o := range out {
		if w.OutputFunc != nil {
			w.OutputFunc(o.Line)
		}
		for s, err := range errMapping {
			if strings.Contains(o.Line, s) {
				e = nonil(e, err)
			}
		}
		e = nonil(e, o.Error)
	}
	return nonil(e, err)
}

// Wait is a convenience function that consumes Vagrant output looking
// for well-known errors.
func Wait(out <-chan *CommandOutput, err error) error {
	return defaultWaiter.Wait(out, err)
}

var errMapping = map[string]error{
	"The box you're attempting to add already exists.":               ErrBoxAlreadyExists,
	"Gem::Requirement::BadRequirementError":                          ErrBoxInvalidVersion,
	"could not be accessed in the remote catalog.":                   ErrBoxNotAvailable,
	"An error occurred while downloading the remote file.":           ErrBoxDownload,
	"VirtualBox is complaining that the kernel module is not loaded": ErrVirtualBox,
}

func nonil(err ...error) error {
	for _, e := range err {
		if e != nil {
			return e
		}
	}
	return nil
}
