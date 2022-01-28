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
	"github.com/openziti/ziti/ziti/cmd/ziti/constants"
	"github.com/pkg/errors"
	"os"
	"strings"
)

const (
	PathSeparator = "/"

	ZitiHomeVarName = "ZITI_HOME"

	ZitiCtrlHostnameVarName = "ZITI_CONTROLLER_HOSTNAME"

	ZitiCtrlRawnameVarName = "ZITI_CONTROLLER_RAWNAME"

	ZitiEdgeRouterHostnameVarName = "ZITI_EDGE_ROUTER_HOSTNAME"

	ZitiEdgeRouterPortVarName = "ZITI_EDGE_ROUTER_PORT"

	ZitiCtrlIdentityCertVarName = "ZITI_CTRL_IDENTITY_CERT"

	ZitiCtrlIdentityServerCertVarName = "ZITI_CTRL_IDENTITY_SERVER_CERT"

	ZitiCtrlIdentityKeyVarName = "ZITI_CTRL_IDENTITY_KEY"

	ZitiCtrlIdentityCAVarName = "ZITI_CTRL_IDENTITY_CA"

	ZitiSigningCertVarName = "ZITI_SIGNING_CERT"

	ZitiSigningKeyVarName = "ZITI_SIGNING_KEY"

	ZitiCtrlListenerHostPortVarName = "ZITI_CTRL_LISTENER_HOST_PORT"

	ZitiCtrlMgmtListenerHostPortVarName = "ZITI_CTRL_MGMT_HOST_PORT"

	ZitiEdgeCtrlListenerHostPortVarName = "ZITI_CTRL_EDGE_LISTENER_HOST_PORT"

	ZitiEdgeCtrlAdvertisedVarName = "ZITI_EDGE_CTRL_ADVERTISED"
)

func HomeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	h := os.Getenv("USERPROFILE") // windows
	if h == "" {
		h = "."
	}
	return strings.ReplaceAll(h, "\\", PathSeparator)
}

func WorkingDir() (string, error) {
	wd, err := os.Getwd()
	if wd == "" || err != nil {
		return "", err
	}

	return strings.ReplaceAll(wd, "\\", PathSeparator), nil
}

func GetZitiHome() (string, error) {

	// Get path from env variable
	retVal := os.Getenv(ZitiHomeVarName)
	if retVal != "" {
		return retVal, nil
	}

	// If not set, create a default path of the current working directory
	workingDir, err := WorkingDir()
	if err != nil {
		return "", err
	}

	err = os.Setenv(ZitiHomeVarName, workingDir)
	if err != nil {
		return "", err
	}

	retVal = os.Getenv(ZitiHomeVarName)

	return retVal, nil
}

func GetZitiIdentityCert() (string, error) {
	hostName, err := os.Hostname()
	if err != nil {
		err := errors.Wrap(err, "Unable to get hostname")
		if err != nil {
			return "", err
		}
	}

	return getOrSetEnvVar(ZitiCtrlIdentityCertVarName, fmt.Sprintf("./%s-client.cert", hostName))
}

func GetZitiIdentityServerCert() (string, error) {
	hostName, err := os.Hostname()
	if err != nil {
		err := errors.Wrap(err, "Unable to get hostname")
		if err != nil {
			return "", err
		}
	}

	return getOrSetEnvVar(ZitiCtrlIdentityServerCertVarName, fmt.Sprintf("./%s-server.pem", hostName))
}

func GetZitiIdentityKey() (string, error) {
	hostName, err := os.Hostname()
	if err != nil {
		err := errors.Wrap(err, "Unable to get hostname")
		if err != nil {
			return "", err
		}
	}

	return getOrSetEnvVar(ZitiCtrlIdentityKeyVarName, fmt.Sprintf("./%s-server.key", hostName))
}

func GetZitiIdentityCA() (string, error) {
	hostName, err := os.Hostname()
	if err != nil {
		err := errors.Wrap(err, "Unable to get hostname")
		if err != nil {
			return "", err
		}
	}

	return getOrSetEnvVar(ZitiCtrlIdentityCAVarName, fmt.Sprintf("./%s-cas.pem", hostName))
}

func GetZitiCtrlListenerHostPort() (string, error) {
	return getOrSetEnvVar(ZitiCtrlListenerHostPortVarName, constants.DefaultZitiControllerListenerHostPort)
}

func GetZitiCtrlMgmtListenerHostPort() (string, error) {
	return getOrSetEnvVar(ZitiCtrlMgmtListenerHostPortVarName, constants.DefaultZitiMgmtControllerListenerHostPort)
}

func GetZitiCtrlHostname() (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		err := errors.Wrap(err, "Unable to get hostname")
		if err != nil {
			return "", err
		}
	}

	return getOrSetEnvVar(ZitiCtrlHostnameVarName, hostname)
}

func GetZitiCtrlRawname() (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		err := errors.Wrap(err, "Unable to get hostname")
		if err != nil {
			return "", err
		}
	}

	defaultZitiCtrlRawName := hostname + "-controller"
	return getOrSetEnvVar(ZitiCtrlRawnameVarName, defaultZitiCtrlRawName)
}

func GetZitiEdgeRouterHostname() (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		err := errors.Wrap(err, "Unable to get hostname")
		if err != nil {
			return "", err
		}
	}
	return getOrSetEnvVar(ZitiEdgeRouterHostnameVarName, hostname)
}

func GetZitiEdgeRouterPort() (string, error) {
	return getOrSetEnvVar(ZitiEdgeRouterPortVarName, constants.DefaultZitiEdgeRouterPort)
}

func GetZitiSigningCert() (string, error) {
	return getOrSetEnvVar(ZitiSigningCertVarName, "./signingCert.cert")
}

func GetZitiSigningKey() (string, error) {
	return getOrSetEnvVar(ZitiSigningKeyVarName, "./signingKey.key")
}

func GetZitiEdgeCtrlListenerHostPort() (string, error) {
	return getOrSetEnvVar(ZitiEdgeCtrlListenerHostPortVarName, constants.DefaultZitiEdgeListenerHostPort)
}

func GetZitiEdgeCtrlAdvertised() (string, error) {
	return getOrSetEnvVar(ZitiEdgeCtrlAdvertisedVarName, "")
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
