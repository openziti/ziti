/*
	Copyright NetFoundry, Inc.

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
	"github.com/pkg/errors"
	"net/url"
	"os"
	"strings"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

const (
	DefaultErrorExitCode = 1

	DefaultWritePermissions = 0760

	ZitiHomeVarName = "ZITI_HOME"

	ZitiPKIVarName = "ZITI_PKI"

	ZitiFabCtrlPortVarName = "ZITI_FAB_CTRL_PORT"

	ZitiCtrlHostnameVarName = "ZITI_CONTROLLER_HOSTNAME"

	ZitiCtrlRawnameVarName = "ZITI_CONTROLLER_RAWNAME"

	ZitiNetworkVarName = "ZITI_NETWORK"

	ZitiDomainSuffixVarName = "ZITI_DOMAIN_SUFFIX"

	ZitiCtrlIntermediateNameVarName = "ZITI_CONTROLLER_INTERMEDIATE_NAME"

	ZitiEdgeCtrlAPIVarName = "ZITI_EDGE_CONTROLLER_API"

	ZitiEdgeCtrlHostnameVarName = "ZITI_EDGE_CONTROLLER_HOSTNAME"

	ZitiEdgeCtrlPortVarName = "ZITI_EDGE_CONTROLLER_PORT"

	ZitiSigningIntermediateNameVarName = "ZITI_SIGNING_INTERMEDIATE_NAME"

	ZitiSigningCertNameVarName = "ZITI_SIGNING_CERT_NAME"

	ZitiFabMgmtPortVarName = "ZITI_FAB_MGMT_PORT"

	ZitiEdgeCtrlIntermediateName = "ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME"
)

type debugError interface {
	DebugError() (msg string, args []interface{})
}

var fatalErrHandler = fatal

// BehaviorOnFatal allows you to override the default behavior when a fatal
// error occurs, which is to call os.Exit(code). You can pass 'panic' as a function
// here if you prefer the panic() over os.Exit(1).
func BehaviorOnFatal(f func(string, int)) {
	fatalErrHandler = f
}

// DefaultBehaviorOnFatal allows you to undo any previous override.  Useful in
// tests.
func DefaultBehaviorOnFatal() {
	fatalErrHandler = fatal
}

// fatal prints the message (if provided) and then exits. If V(2) or greater,
// glog.Fatal is invoked for extended information.
func fatal(msg string, code int) {
	/*
		if glog.V(2) {
			glog.FatalDepth(2, msg)
		}
	*/
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

// checkErrWithPrefix works like CheckErr, but adds a caller-defined prefix to non-nil errors
func checkErrWithPrefix(prefix string, err error) {
	checkErr(prefix, err, fatalErrHandler)
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
	/*
		if debugErr, ok := err.(debugError); ok {
			glog.V(4).Infof(debugErr.DebugError())
		}
		status, isStatus := err.(kerrors.APIStatus)
		switch {
		case isStatus:
			switch s := status.Status(); {
			case s.Reason == unversioned.StatusReasonUnauthorized:
				return fmt.Sprintf("error: You must be logged in to the server (%s)", s.Message), true
			case len(s.Reason) > 0:
				return fmt.Sprintf("Error from server (%s): %s", s.Reason, err.Error()), true
			default:
				return fmt.Sprintf("Error from server: %s", err.Error()), true
			}
		case kerrors.IsUnexpectedObjectError(err):
			return fmt.Sprintf("Server returned an unexpected response: %s", err.Error()), true
		}
	*/
	switch t := err.(type) {
	case *url.Error:
		glog.V(4).Infof("Connection error: %s %s: %v", t.Op, t.URL, t.Err)
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

func UsageError(cmd *cobra.Command, format string, args ...interface{}) error {
	msg := fmt.Sprintf(format, args...)
	return fmt.Errorf("%s\nSee '%s -h' for help and examples.", msg, cmd.CommandPath())
}

func HomeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	h := os.Getenv("USERPROFILE") // windows
	if h == "" {
		h = "."
	}
	return h
}

func GetZitiHome() (string, error) {

	// Get path from env variable
	retVal := os.Getenv(ZitiHomeVarName)
	if retVal != "" {
		return retVal, nil
	}

	// If not set, create a default path
	hostname, err := os.Hostname()
	if err != nil {
		return "", err
	}

	homePath := HomeDir()

	pathSep := string(os.PathSeparator)
	err = os.Setenv(ZitiHomeVarName, homePath+pathSep+".ziti"+pathSep+"quickstart"+pathSep+hostname)
	if err != nil {
		return "", err
	}

	retVal = os.Getenv(ZitiHomeVarName)

	return retVal, nil
}

func GetZitiPKI() (string, error) {
	zitiHome, err := GetZitiHome()
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+ZitiHomeVarName)
		if err != nil {
			return "", err
		}
	}
	return getOrSetEnvVar(ZitiPKIVarName, zitiHome+string(os.PathSeparator)+"pki")
}

func GetZitiCtrlIntermediateName() (string, error) {
	zitiCtrlHostname, err := GetZitiCtrlHostname()
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+ZitiCtrlHostnameVarName)
		if err != nil {
			return "", err
		}
	}

	return getOrSetEnvVar(ZitiCtrlIntermediateNameVarName, zitiCtrlHostname+"-intermediate")
}

func GetZitiEdgeCtrlHostname() (string, error) {
	zitiNetwork, err := GetZitiNetwork()
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+ZitiNetworkVarName)
		if err != nil {
			return "", err
		}
	}

	return getOrSetEnvVar(ZitiEdgeCtrlHostnameVarName, zitiNetwork)
}

func GetZitiEdgeCtrlPort() (string, error) {
	return getOrSetEnvVar(ZitiEdgeCtrlPortVarName, "1280")
}

func GetZitiEdgeControllerAPI() (string, error) {
	zitiEdgeCtrlHostname, err := GetZitiEdgeCtrlHostname()
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+ZitiCtrlHostnameVarName)
		if err != nil {
			return "", err
		}
	}

	zitiEdgeCtrlPort, err := GetZitiEdgeCtrlPort()
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+ZitiEdgeCtrlPortVarName)
		if err != nil {
			return "", err
		}
	}

	return getOrSetEnvVar(ZitiEdgeCtrlAPIVarName, zitiEdgeCtrlHostname+":"+zitiEdgeCtrlPort)
}

func GetZitiSigningCertName() (string, error) {
	zitiNetwork, err := GetZitiNetwork()
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+ZitiNetworkVarName)
		if err != nil {
			return "", err
		}
	}

	return getOrSetEnvVar(ZitiSigningCertNameVarName, zitiNetwork+"-signing")
}

func GetZitiSigningIntermediateName() (string, error) {
	zitiSigningCertName, err := GetZitiSigningCertName()
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+ZitiSigningCertNameVarName)
		if err != nil {
			return "", err
		}
	}

	return getOrSetEnvVar(ZitiSigningIntermediateNameVarName, zitiSigningCertName+"-intermediate")
}

func GetZitiFabCtrlPort() (string, error) {
	return getOrSetEnvVar(ZitiFabCtrlPortVarName, "6262")
}

func GetZitiCtrlHostname() (string, error) {
	zitiNetwork, err := GetZitiNetwork()
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+ZitiNetworkVarName)
		if err != nil {
			return "", err
		}
	}

	return getOrSetEnvVar(ZitiCtrlHostnameVarName, zitiNetwork)
}

func GetZitiCtrlRawname() (string, error) {
	zitiNetwork, err := GetZitiNetwork()
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+ZitiNetworkVarName)
		if err != nil {
			return "", err
		}
	}

	defaultZitiCtrlRawName := zitiNetwork + "-controller"
	return getOrSetEnvVar(ZitiCtrlRawnameVarName, defaultZitiCtrlRawName)
}

func GetZitiNetwork() (string, error) {
	hostName, err := os.Hostname()
	if err != nil {
		err := errors.Wrap(err, "Unable to get hostname")
		if err != nil {
			return "", err
		}
	}

	return getOrSetEnvVar(ZitiNetworkVarName, hostName)
}

func GetZitiFabMgmtPort() (string, error) {
	return getOrSetEnvVar(ZitiFabMgmtPortVarName, "10000")
}

func getOrSetEnvVar(envVarName string, defaultValue string) (string, error) {

	// Get path from env variable
	retVal := os.Getenv(envVarName)
	if retVal != "" {
		return retVal, nil
	}

	err := os.Setenv(envVarName, defaultValue)
	if err != nil {
		return "", err
	}

	retVal = os.Getenv(envVarName)

	return retVal, nil
}

func GetZitiEdgeCtrlIntermediateName() (string, error) {
	zitiEdgeCtrlHostname, err := GetZitiEdgeCtrlHostname()
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+ZitiEdgeCtrlHostnameVarName)
		if err != nil {
			return "", err
		}
	}

	return getOrSetEnvVar(ZitiEdgeCtrlIntermediateName, zitiEdgeCtrlHostname+"-intermediate")
}

func JFrogAPIKey() string {
	if h := os.Getenv("JFROG_API_KEY"); h != "" {
		return h
	}
	panic(fmt.Sprintf("ERROR: the JFROG_API_KEY env variable has not been set"))
}
