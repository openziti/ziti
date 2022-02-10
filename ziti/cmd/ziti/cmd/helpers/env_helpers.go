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

type EnvVariableMetaData struct {
	OS                                           string
	PathSeparator                                string
	ZitiHomeVarName                              string
	ZitiCtrlNameVarName                          string
	ZitiEdgeRouterHostnameVarName                string
	ZitiEdgeRouterPortVarName                    string
	ZitiEdgeCtrlIdentityCertVarName              string
	ZitiEdgeCtrlIdentityServerCertVarName        string
	ZitiEdgeCtrlIdentityKeyVarName               string
	ZitiEdgeCtrlIdentityCAVarName                string
	ZitiCtrlIdentityCertVarName                  string
	ZitiCtrlIdentityServerCertVarName            string
	ZitiCtrlIdentityKeyVarName                   string
	ZitiCtrlIdentityCAVarName                    string
	ZitiSigningCertVarName                       string
	ZitiSigningKeyVarName                        string
	ZitiCtrlListenerHostPortVarName              string
	ZitiCtrlMgmtListenerHostPortVarName          string
	ZitiEdgeCtrlListenerHostPortVarName          string
	ZitiEdgeCtrlAdvertisedHostPortVarName        string
	ZitiRouterIdentityCertVarName                string
	ZitiRouterIdentityServerCertVarName          string
	ZitiRouterIdentityKeyVarName                 string
	ZitiRouterIdentityCAVarName                  string
	ZitiHomeVarDescription                       string
	ZitiCtrlNameVarDescription                   string
	ZitiEdgeRouterHostnameVarDescription         string
	ZitiEdgeRouterPortVarDescription             string
	ZitiEdgeCtrlIdentityCertVarDescription       string
	ZitiEdgeCtrlIdentityServerCertVarDescription string
	ZitiEdgeCtrlIdentityKeyVarDescription        string
	ZitiEdgeCtrlIdentityCAVarDescription         string
	ZitiCtrlIdentityCertVarDescription           string
	ZitiCtrlIdentityServerCertVarDescription     string
	ZitiCtrlIdentityKeyVarDescription            string
	ZitiCtrlIdentityCAVarDescription             string
	ZitiSigningCertVarDescription                string
	ZitiSigningKeyVarDescription                 string
	ZitiCtrlListenerHostPortVarDescription       string
	ZitiCtrlMgmtListenerHostPortVarDescription   string
	ZitiEdgeCtrlListenerHostPortVarDescription   string
	ZitiEdgeCtrlAdvertisedHostPortVarDescription string
	ZitiRouterIdentityCertVarDescription         string
	ZitiRouterIdentityServerCertVarDescription   string
	ZitiRouterIdentityKeyVarDescription          string
	ZitiRouterIdentityCAVarDescription           string
}

var EnvVariableDetails = EnvVariableMetaData{
	PathSeparator:                                "/",
	ZitiHomeVarName:                              "ZITI_HOME",
	ZitiHomeVarDescription:                       "Root home directory for Ziti related files",
	ZitiCtrlNameVarName:                          "ZITI_CONTROLLER_NAME",
	ZitiCtrlNameVarDescription:                   "The name of the Ziti Controller",
	ZitiEdgeRouterHostnameVarName:                "ZITI_EDGE_ROUTER_HOSTNAME",
	ZitiEdgeRouterHostnameVarDescription:         "Hostname of the Ziti Edge Router",
	ZitiEdgeRouterPortVarName:                    "ZITI_EDGE_ROUTER_PORT",
	ZitiEdgeRouterPortVarDescription:             "Port of the Ziti Edge Router",
	ZitiEdgeCtrlIdentityCertVarName:              "ZITI_EDGE_CTRL_IDENTITY_CERT",
	ZitiEdgeCtrlIdentityCertVarDescription:       "Path to Identity Cert for Ziti Edge Controller",
	ZitiEdgeCtrlIdentityServerCertVarName:        "ZITI_EDGE_CTRL_IDENTITY_SERVER_CERT",
	ZitiEdgeCtrlIdentityServerCertVarDescription: "Path to Identity Server Cert for Ziti Edge Controller",
	ZitiEdgeCtrlIdentityKeyVarName:               "ZITI_EDGE_CTRL_IDENTITY_KEY",
	ZitiEdgeCtrlIdentityKeyVarDescription:        "Path to Identity Key for Ziti Edge Controller",
	ZitiEdgeCtrlIdentityCAVarName:                "ZITI_EDGE_CTRL_IDENTITY_CA",
	ZitiEdgeCtrlIdentityCAVarDescription:         "Path to Identity CA for Ziti Edge Controller",
	ZitiCtrlIdentityCertVarName:                  "ZITI_CTRL_IDENTITY_CERT",
	ZitiCtrlIdentityCertVarDescription:           "Path to Identity Cert for Ziti Controller",
	ZitiCtrlIdentityServerCertVarName:            "ZITI_CTRL_IDENTITY_SERVER_CERT",
	ZitiCtrlIdentityServerCertVarDescription:     "Path to Identity Server Cert for Ziti Controller",
	ZitiCtrlIdentityKeyVarName:                   "ZITI_CTRL_IDENTITY_KEY",
	ZitiCtrlIdentityKeyVarDescription:            "Path to Identity Key for Ziti Controller",
	ZitiCtrlIdentityCAVarName:                    "ZITI_CTRL_IDENTITY_CA",
	ZitiCtrlIdentityCAVarDescription:             "Path to Identity CA for Ziti Controller",
	ZitiSigningCertVarName:                       "ZITI_SIGNING_CERT",
	ZitiSigningCertVarDescription:                "Path to the Ziti Signing Cert",
	ZitiSigningKeyVarName:                        "ZITI_SIGNING_KEY",
	ZitiSigningKeyVarDescription:                 "Path to the Ziti Signing Key",
	ZitiRouterIdentityCertVarName:                "ZITI_ROUTER_IDENTITY_CERT",
	ZitiRouterIdentityCertVarDescription:         "Path to Identity Cert for Ziti Router",
	ZitiRouterIdentityServerCertVarName:          "ZITI_ROUTER_IDENTITY_SERVER_CERT",
	ZitiRouterIdentityServerCertVarDescription:   "Path to Identity Server Cert for Ziti Router",
	ZitiRouterIdentityKeyVarName:                 "ZITI_ROUTER_IDENTITY_KEY",
	ZitiRouterIdentityKeyVarDescription:          "Path to Identity Key for Ziti Router",
	ZitiRouterIdentityCAVarName:                  "ZITI_ROUTER_IDENTITY_CA",
	ZitiRouterIdentityCAVarDescription:           "Path to Identity CA for Ziti Router",
	ZitiCtrlListenerHostPortVarName:              "ZITI_CTRL_LISTENER_HOST_PORT",
	ZitiCtrlListenerHostPortVarDescription:       "Host and port of the Ziti Controller Listener",
	ZitiCtrlMgmtListenerHostPortVarName:          "ZITI_CTRL_MGMT_HOST_PORT",
	ZitiCtrlMgmtListenerHostPortVarDescription:   "Host and port of the Ziti Controller Management Listener",
	ZitiEdgeCtrlListenerHostPortVarName:          "ZITI_CTRL_EDGE_LISTENER_HOST_PORT",
	ZitiEdgeCtrlListenerHostPortVarDescription:   "Host and port of the Ziti Edge Controller Listener",
	ZitiEdgeCtrlAdvertisedHostPortVarName:        "ZITI_EDGE_CTRL_ADVERTISED_HOST_PORT",
	ZitiEdgeCtrlAdvertisedHostPortVarDescription: "Host and port of the Ziti Edge Controller API",
}

func HomeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	h := os.Getenv("USERPROFILE") // windows
	if h == "" {
		h = "."
	}
	return strings.ReplaceAll(h, "\\", EnvVariableDetails.PathSeparator)
}

func WorkingDir() (string, error) {
	wd, err := os.Getwd()
	if wd == "" || err != nil {
		return "", err
	}

	return strings.ReplaceAll(wd, "\\", EnvVariableDetails.PathSeparator), nil
}

func GetZitiHome() (string, error) {

	// Get path from env variable
	retVal := os.Getenv(EnvVariableDetails.ZitiHomeVarName)

	if retVal == "" {
		// If not set, create a default path of the current working directory
		workingDir, err := WorkingDir()
		if err != nil {
			return "", err
		}

		err = os.Setenv(EnvVariableDetails.ZitiHomeVarName, workingDir)
		if err != nil {
			return "", err
		}

		retVal = os.Getenv(EnvVariableDetails.ZitiHomeVarName)
	}

	return strings.ReplaceAll(retVal, "\\", EnvVariableDetails.PathSeparator), nil
}

func GetZitiCtrlIdentityCert() (string, error) {
	workingDir, err := WorkingDir()
	if err != nil {
		return "", err
	}

	controllerName, err := GetZitiCtrlName()
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+EnvVariableDetails.ZitiCtrlNameVarName)
		if err != nil {
			return "", err
		}
	}

	return getValueOrSetAndGetDefault(EnvVariableDetails.ZitiCtrlIdentityCertVarName, fmt.Sprintf("%s/%s-client.cert", workingDir, controllerName), false)
}

func GetZitiCtrlIdentityServerCert() (string, error) {
	workingDir, err := WorkingDir()
	if err != nil {
		return "", err
	}

	controllerName, err := GetZitiCtrlName()
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+EnvVariableDetails.ZitiCtrlNameVarName)
		if err != nil {
			return "", err
		}
	}

	return getValueOrSetAndGetDefault(EnvVariableDetails.ZitiCtrlIdentityServerCertVarName, fmt.Sprintf("%s/%s-server.pem", workingDir, controllerName), false)
}

func GetZitiCtrlIdentityKey() (string, error) {
	workingDir, err := WorkingDir()
	if err != nil {
		return "", err
	}

	controllerName, err := GetZitiCtrlName()
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+EnvVariableDetails.ZitiCtrlNameVarName)
		if err != nil {
			return "", err
		}
	}

	return getValueOrSetAndGetDefault(EnvVariableDetails.ZitiCtrlIdentityKeyVarName, fmt.Sprintf("%s/%s-server.key", workingDir, controllerName), false)
}

func GetZitiCtrlIdentityCA() (string, error) {
	workingDir, err := WorkingDir()
	if err != nil {
		return "", err
	}

	controllerName, err := GetZitiCtrlName()
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+EnvVariableDetails.ZitiCtrlNameVarName)
		if err != nil {
			return "", err
		}
	}

	return getValueOrSetAndGetDefault(EnvVariableDetails.ZitiCtrlIdentityCAVarName, fmt.Sprintf("%s/%s-cas.pem", workingDir, controllerName), false)
}

func GetZitiRouterIdentityCert(routerName string, forceSet bool) (string, error) {
	workingDir, err := WorkingDir()
	if err != nil {
		return "", err
	}

	return getValueOrSetAndGetDefault(EnvVariableDetails.ZitiRouterIdentityCertVarName, fmt.Sprintf("%s/%s-client.cert", workingDir, routerName), forceSet)
}

func GetZitiRouterIdentityServerCert(routerName string, forceSet bool) (string, error) {
	workingDir, err := WorkingDir()
	if err != nil {
		return "", err
	}

	return getValueOrSetAndGetDefault(EnvVariableDetails.ZitiRouterIdentityServerCertVarName, fmt.Sprintf("%s/%s-server.pem", workingDir, routerName), forceSet)
}

func GetZitiRouterIdentityKey(routerName string, forceSet bool) (string, error) {
	workingDir, err := WorkingDir()
	if err != nil {
		return "", err
	}

	return getValueOrSetAndGetDefault(EnvVariableDetails.ZitiRouterIdentityKeyVarName, fmt.Sprintf("%s/%s-server.key", workingDir, routerName), forceSet)
}

func GetZitiRouterIdentityCA(routerName string, forceSet bool) (string, error) {
	workingDir, err := WorkingDir()
	if err != nil {
		return "", err
	}

	return getValueOrSetAndGetDefault(EnvVariableDetails.ZitiRouterIdentityCAVarName, fmt.Sprintf("%s/%s-cas.pem", workingDir, routerName), forceSet)
}

func GetZitiEdgeIdentityCert() (string, error) {
	workingDir, err := WorkingDir()
	if err != nil {
		return "", err
	}

	controllerName, err := GetZitiCtrlName()
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+EnvVariableDetails.ZitiCtrlNameVarName)
		if err != nil {
			return "", err
		}
	}

	return getValueOrSetAndGetDefault(EnvVariableDetails.ZitiEdgeCtrlIdentityCertVarName, fmt.Sprintf("%s/%s-client.cert", workingDir, controllerName), false)
}

func GetZitiEdgeIdentityServerCert() (string, error) {
	workingDir, err := WorkingDir()
	if err != nil {
		return "", err
	}

	controllerName, err := GetZitiCtrlName()
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+EnvVariableDetails.ZitiCtrlNameVarName)
		if err != nil {
			return "", err
		}
	}

	return getValueOrSetAndGetDefault(EnvVariableDetails.ZitiEdgeCtrlIdentityServerCertVarName, fmt.Sprintf("%s/%s-server.pem", workingDir, controllerName), false)
}

func GetZitiEdgeIdentityKey() (string, error) {
	workingDir, err := WorkingDir()
	if err != nil {
		return "", err
	}

	controllerName, err := GetZitiCtrlName()
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+EnvVariableDetails.ZitiCtrlNameVarName)
		if err != nil {
			return "", err
		}
	}

	return getValueOrSetAndGetDefault(EnvVariableDetails.ZitiEdgeCtrlIdentityKeyVarName, fmt.Sprintf("%s/%s-server.key", workingDir, controllerName), false)
}

func GetZitiEdgeIdentityCA() (string, error) {
	workingDir, err := WorkingDir()
	if err != nil {
		return "", err
	}

	controllerName, err := GetZitiCtrlName()
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+EnvVariableDetails.ZitiCtrlNameVarName)
		if err != nil {
			return "", err
		}
	}

	return getValueOrSetAndGetDefault(EnvVariableDetails.ZitiEdgeCtrlIdentityCAVarName, fmt.Sprintf("%s/%s-cas.pem", workingDir, controllerName), false)
}

func GetZitiCtrlListenerHostPort() (string, error) {
	return getValueOrSetAndGetDefault(EnvVariableDetails.ZitiCtrlListenerHostPortVarName, constants.DefaultZitiControllerListenerHostPort, false)
}

func GetZitiCtrlMgmtListenerHostPort() (string, error) {
	return getValueOrSetAndGetDefault(EnvVariableDetails.ZitiCtrlMgmtListenerHostPortVarName, constants.DefaultZitiMgmtControllerListenerHostPort, false)
}

func GetZitiCtrlName() (string, error) {
	return getValueOrSetAndGetDefault(EnvVariableDetails.ZitiCtrlNameVarName, constants.DefaultZitiControllerName, false)
}

func GetZitiEdgeRouterHostname() (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		err := errors.Wrap(err, "Unable to get hostname")
		if err != nil {
			return "", err
		}
	}
	return getValueOrSetAndGetDefault(EnvVariableDetails.ZitiEdgeRouterHostnameVarName, hostname, false)
}

func GetZitiEdgeRouterPort() (string, error) {
	return getValueOrSetAndGetDefault(EnvVariableDetails.ZitiEdgeRouterPortVarName, constants.DefaultZitiEdgeRouterPort, false)
}

func GetZitiSigningCert() (string, error) {
	workingDir, err := WorkingDir()
	if err != nil {
		return "", err
	}

	return getValueOrSetAndGetDefault(EnvVariableDetails.ZitiSigningCertVarName, fmt.Sprintf("%s/signingCert.cert", workingDir), false)
}

func GetZitiSigningKey() (string, error) {
	workingDir, err := WorkingDir()
	if err != nil {
		return "", err
	}

	return getValueOrSetAndGetDefault(EnvVariableDetails.ZitiSigningKeyVarName, fmt.Sprintf("%s/signingKey.key", workingDir), false)
}

func GetZitiEdgeCtrlListenerHostPort() (string, error) {
	return getValueOrSetAndGetDefault(EnvVariableDetails.ZitiEdgeCtrlListenerHostPortVarName, constants.DefaultZitiEdgeListenerHostPort, false)
}

func GetZitiEdgeCtrlAdvertisedHostPort() (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		err := errors.Wrap(err, "Unable to get hostname")
		if err != nil {
			return "", err
		}
	}

	return getValueOrSetAndGetDefault(EnvVariableDetails.ZitiEdgeCtrlAdvertisedHostPortVarName, hostname+":"+constants.DefaultZitiEdgeAPIPort, false)
}

func getValueOrSetAndGetDefault(envVarName string, defaultValue string, forceDefault bool) (string, error) {
	retVal := ""
	if !forceDefault {
		// Get path from env variable
		retVal = os.Getenv(envVarName)
		if retVal != "" {
			return retVal, nil
		}
	}

	err := os.Setenv(envVarName, defaultValue)
	if err != nil {
		return "", err
	}

	retVal = os.Getenv(envVarName)

	return retVal, nil
}
