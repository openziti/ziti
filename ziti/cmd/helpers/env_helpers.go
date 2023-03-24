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
	edge "github.com/openziti/edge/controller/config"
	"github.com/openziti/ziti/ziti/constants"
	"github.com/pkg/errors"
	"os"
	"strconv"
	"strings"
	"time"
)

func HomeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	h := os.Getenv("USERPROFILE") // windows
	if h == "" {
		h = "."
	}
	return NormalizePath(h)
}

func WorkingDir() (string, error) {
	wd, err := os.Getwd()
	if wd == "" || err != nil {
		return "", err
	}

	return NormalizePath(wd), nil
}

func GetZitiHome() (string, error) {

	// Get path from env variable
	retVal := os.Getenv(constants.ZitiHomeVarName)

	if retVal == "" {
		// If not set, create a default path of the current working directory
		workingDir, err := WorkingDir()
		if err != nil {
			// If there is an error just use .
			workingDir = "."
		}

		err = os.Setenv(constants.ZitiHomeVarName, workingDir)
		if err != nil {
			return "", err
		}

		retVal = os.Getenv(constants.ZitiHomeVarName)
	}

	return NormalizePath(retVal), nil
}

func HostnameOrNetworkName() string {
	val := os.Getenv("ZITI_NETWORK_NAME")
	if val == "" {
		h, err := os.Hostname()
		if err != nil {
			return "localhost"
		}
		return h
	}
	return val
}

func GetCtrlListenerAddress() (string, error) {
	return getValueOrSetAndGetDefault(constants.CtrlListenerAddressVarName, constants.DefaultCtrlListenerAddress)
}

func GetCtrlListenerPort() (string, error) {
	return getValueOrSetAndGetDefault(constants.CtrlListenerPortVarName, constants.DefaultCtrlListenerPort)
}

func GetCtrlEdgeApiAddress() (string, error) {
	// Get the controller's edge advertised hostname to use as the default
	defaultHostname, err := GetCtrlEdgeAdvertisedAddress()
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+constants.CtrlEdgeAdvertisedAddressVarName)
		if err != nil {
			return "", err
		}
	}

	return getValueOrSetAndGetDefault(constants.CtrlEdgeApiAddressVarName, defaultHostname)
}

func GetCtrlEdgeApiPort() (string, error) {
	// Get the controller's edge advertised port to use as the default
	defaultPort, err := GetCtrlEdgeAdvertisedPort()
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+constants.CtrlEdgeAdvertisedPortVarName)
		if err != nil {
			return "", err
		}
	}

	return getValueOrSetAndGetDefault(constants.CtrlEdgeApiPortVarName, defaultPort)
}

func GetCtrlEdgeInterfaceAddress() (string, error) {
	// Get the controller's listener hostname to use as the default
	defaultHostname, err := GetCtrlListenerAddress()
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+constants.CtrlListenerAddressVarName)
		if err != nil {
			return "", err
		}
	}

	return getValueOrSetAndGetDefault(constants.CtrlEdgeInterfaceAddressVarName, defaultHostname)
}

func GetCtrlEdgeInterfacePort() (string, error) {
	// Get the controller's edge advertised port to use as the default
	defaultPort, err := GetCtrlEdgeAdvertisedPort()
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+constants.CtrlEdgeAdvertisedPortVarName)
		if err != nil {
			return "", err
		}
	}

	return getValueOrSetAndGetDefault(constants.CtrlEdgeInterfacePortVarName, defaultPort)
}

func GetCtrlEdgeAdvertisedAddress() (string, error) {

	// Use hostname if edge advertised address not set
	hostname, err := os.Hostname()
	if err != nil {
		err := errors.Wrap(err, "Unable to get hostname")
		if err != nil {
			return "", err
		}
	}

	return getValueOrSetAndGetDefault(constants.CtrlEdgeAdvertisedAddressVarName, hostname)
}

func GetCtrlEdgeAdvertisedPort() (string, error) {
	return getValueOrSetAndGetDefault(constants.CtrlEdgeAdvertisedPortVarName, constants.DefaultCtrlEdgeAdvertisedPort)
}

func GetZitiEdgeRouterPort() (string, error) {
	return getValueOrSetAndGetDefault(constants.ZitiEdgeRouterPortVarName, constants.DefaultZitiEdgeRouterPort)
}

func GetZitiEdgeRouterListenerBindPort() (string, error) {
	return getValueOrSetAndGetDefault(constants.ZitiEdgeRouterListenerBindPortVarName, constants.DefaultZitiEdgeRouterListenerBindPort)
}

func GetCtrlEdgeIdentityEnrollmentDuration() (time.Duration, error) {
	retVal, err := getValueOrSetAndGetDefault(constants.CtrlEdgeIdentityEnrollmentDurationVarName, strconv.FormatInt(int64(edge.DefaultEdgeEnrollmentDuration.Minutes()), 10))
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+constants.CtrlEdgeIdentityEnrollmentDurationVarDescription)
		if err != nil {
			return edge.DefaultEdgeEnrollmentDuration, err
		}
	}
	retValInt, err := strconv.Atoi(retVal)
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+constants.CtrlEdgeIdentityEnrollmentDurationVarDescription)
		if err != nil {
			return edge.DefaultEdgeEnrollmentDuration, err
		}
	}

	return time.Duration(retValInt) * time.Minute, nil
}

func GetCtrlEdgeRouterEnrollmentDuration() (time.Duration, error) {
	retVal, err := getValueOrSetAndGetDefault(constants.CtrlEdgeRouterEnrollmentDurationVarName, strconv.FormatInt(int64(edge.DefaultEdgeEnrollmentDuration.Minutes()), 10))
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+constants.CtrlEdgeRouterEnrollmentDurationVarDescription)
		if err != nil {
			return edge.DefaultEdgeEnrollmentDuration, err
		}
	}
	retValInt, err := strconv.Atoi(retVal)
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+constants.CtrlEdgeRouterEnrollmentDurationVarDescription)
		if err != nil {
			return edge.DefaultEdgeEnrollmentDuration, err
		}
	}

	//fmt.Println("Router Duration: " + retVal + " - " + (time.Duration(retValInt) * time.Minute).String())
	return time.Duration(retValInt) * time.Minute, nil
}

func getValueOrSetAndGetDefault(envVarName string, defaultValue string) (string, error) {
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

// NormalizePath replaces windows \ with / which windows allows for
func NormalizePath(input string) string {
	return strings.ReplaceAll(input, "\\", "/")
}
