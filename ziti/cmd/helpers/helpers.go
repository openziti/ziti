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

package helpers

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/pkg/errors"
	"net/url"
	"os"
	"strings"
)

const (
	DefaultErrorExitCode = 1
)

var fatalErrHandler = fatal

// fatal prints the message (if provided) and then exits.
func fatal(msg string, code int) {
	if len(msg) > 0 {
		// add newline if needed
		if !strings.HasSuffix(msg, "\n") {
			msg += "\n"
		}
		fmt.Fprint(os.Stderr, msg)
	}
	os.Exit(code)
}

// ErrExit may be passed to CheckError to instruct it to output nothing but exit with
// status code 1.
var ErrExit = fmt.Errorf("exit")

// CheckErr prints a user friendly error to STDERR and exits with a non-zero
// exit code. Unrecognized errors will be printed with an "error: " prefix.
//
// This method is generic to the command in use and may be used by non-Kubectl
// commands.
func CheckErr(err error) {
	checkErr("", err, fatalErrHandler)
}

// checkErr formats a given error as a string and calls the passed handleErr
// func with that string and an kubectl exit code.
func checkErr(prefix string, err error, handleErr func(string, int)) {
	// unwrap aggregates of 1
	/*
		if agg, ok := err.(utilerrors.Aggregate); ok && len(agg.Errors()) == 1 {
			err = agg.Errors()[0]
		}
	*/

	switch {
	case err == nil:
		return
	case err == ErrExit:
		handleErr("", DefaultErrorExitCode)
		return
	/*
		case kerrors.IsInvalid(err):
			details := err.(*kerrors.StatusError).Status().Details
			s := fmt.Sprintf("%sThe %s %q is invalid", prefix, details.Kind, details.Name)
			if len(details.Causes) > 0 {
				errs := statusCausesToAggrError(details.Causes)
				handleErr(MultilineError(s+": ", errs), DefaultErrorExitCode)
			} else {
				handleErr(s, DefaultErrorExitCode)
			}
		case clientcmd.IsConfigurationInvalid(err):
			handleErr(MultilineError(fmt.Sprintf("%sError in configuration: ", prefix), err), DefaultErrorExitCode)
	*/
	default:
		switch err := err.(type) {
		/*
			case *meta.NoResourceMatchError:
				switch {
				case len(err.PartialResource.Group) > 0 && len(err.PartialResource.Version) > 0:
					handleErr(fmt.Sprintf("%sthe server doesn't have a resource type %q in group %q and version %q", prefix, err.PartialResource.Resource, err.PartialResource.Group, err.PartialResource.Version), DefaultErrorExitCode)
				case len(err.PartialResource.Group) > 0:
					handleErr(fmt.Sprintf("%sthe server doesn't have a resource type %q in group %q", prefix, err.PartialResource.Resource, err.PartialResource.Group), DefaultErrorExitCode)
				case len(err.PartialResource.Version) > 0:
					handleErr(fmt.Sprintf("%sthe server doesn't have a resource type %q in version %q", prefix, err.PartialResource.Resource, err.PartialResource.Version), DefaultErrorExitCode)
				default:
					handleErr(fmt.Sprintf("%sthe server doesn't have a resource type %q", prefix, err.PartialResource.Resource), DefaultErrorExitCode)
				}
			case utilerrors.Aggregate:
				handleErr(MultipleErrors(prefix, err.Errors()), DefaultErrorExitCode)
			case utilexec.ExitError:
				// do not print anything, only terminate with given error
				handleErr("", err.ExitStatus())
		*/
		default: // for any other error type
			msg, ok := StandardErrorMessage(err)
			if !ok {
				msg = err.Error()
				if !strings.HasPrefix(msg, "error: ") {
					msg = fmt.Sprintf("error: %s", msg)
				}
			}
			handleErr(msg, DefaultErrorExitCode)
		}
	}
}

// StandardErrorMessage translates common errors into a human readable message, or returns
// false if the error is not one of the recognized types. It may also log extended
// information to glog.
//
// This method is generic to the command in use and may be used by non-Kubectl
// commands.
func StandardErrorMessage(err error) (string, bool) {
	switch t := err.(type) {
	case *url.Error:
		pfxlog.Logger().Infof("Connection error: %s %s: %v", t.Op, t.URL, t.Err)
		switch {
		case strings.Contains(t.Err.Error(), "connection refused"):
			host := t.URL
			if server, err := url.Parse(t.URL); err == nil {
				host = server.Host
			}
			return fmt.Sprintf("The connection to the server %s was refused - did you specify the right host or port?", host), true
		}
		return fmt.Sprintf("Unable to connect to the server: %v", t.Err), true
	}
	return "", false
}

func JFrogAPIKey() string {
	if h := os.Getenv("JFROG_API_KEY"); h != "" {
		return h
	}
	panic(errors.New("ERROR: the JFROG_API_KEY env variable has not been set"))
}
