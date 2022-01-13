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
	"github.com/pkg/errors"
	"os"
)

const (
	PathSeparator = "/"

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

	ZitiEdgeCtrlIntermediateNameVarName = "ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME"

	ZitiEdgeRouterHostnameVarName = "ZITI_EDGE_ROUTER_HOSTNAME"

	ZitiEdgeRouterPortVarName = "ZITI_EDGE_ROUTER_PORT"
)

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

	err = os.Setenv(ZitiHomeVarName, homePath+PathSeparator+".ziti"+PathSeparator+"quickstart"+PathSeparator+hostname)
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
	return getOrSetEnvVar(ZitiPKIVarName, zitiHome+PathSeparator+"pki")
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

func GetZitiEdgeCtrlIntermediateName() (string, error) {
	zitiEdgeCtrlHostname, err := GetZitiEdgeCtrlHostname()
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+ZitiEdgeCtrlHostnameVarName)
		if err != nil {
			return "", err
		}
	}

	return getOrSetEnvVar(ZitiEdgeCtrlIntermediateNameVarName, zitiEdgeCtrlHostname+"-intermediate")
}

func GetZitiEdgeRouterHostname() (string, error) {
	zitiNetwork, err := GetZitiNetwork()
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+ZitiNetworkVarName)
		if err != nil {
			return "", err
		}
	}
	return getOrSetEnvVar(ZitiEdgeRouterHostnameVarName, zitiNetwork)
}

func GetZitiEdgeRouterPort() (string, error) {
	return getOrSetEnvVar(ZitiEdgeRouterPortVarName, "3022")
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
