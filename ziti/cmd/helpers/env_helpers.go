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
			return "", err
		}

		err = os.Setenv(constants.ZitiHomeVarName, workingDir)
		if err != nil {
			return "", err
		}

		retVal = os.Getenv(constants.ZitiHomeVarName)
	}

	return NormalizePath(retVal), nil
}

func GetZitiCtrlAdvertisedAddress() (string, error) {

	// Use External DNS if set
	extDNS := os.Getenv(constants.ExternalDNSVarName)
	if extDNS != "" {
		return extDNS, nil
	}

	// Use hostname if external DNS and advertised address not set
	hostname, err := os.Hostname()
	if err != nil {
		err := errors.Wrap(err, "Unable to get hostname")
		if err != nil {
			return "", err
		}
	}

	return getValueOrSetAndGetDefault(constants.ZitiCtrlAdvertisedAddressVarName, hostname)
}

func GetZitiCtrlPort() (string, error) {
	return getValueOrSetAndGetDefault(constants.ZitiCtrlPortVarName, constants.DefaultZitiControllerPort)
}

func GetZitiCtrlListenerAddress() (string, error) {
	return getValueOrSetAndGetDefault(constants.ZitiCtrlListenerAddressVarName, constants.DefaultZitiControllerListenerAddress)
}

func GetZitiCtrlName() (string, error) {
	return getValueOrSetAndGetDefault(constants.ZitiCtrlNameVarName, constants.DefaultZitiControllerName)
}

func GetZitiEdgeRouterPort() (string, error) {
	return getValueOrSetAndGetDefault(constants.ZitiEdgeRouterPortVarName, constants.DefaultZitiEdgeRouterPort)
}

func GetZitiEdgeRouterListenerBindPort() (string, error) {
	return getValueOrSetAndGetDefault(constants.ZitiEdgeRouterListenerBindPortVarName, constants.DefaultZitiEdgeRouterListenerBindPort)
}

func GetZitiEdgeCtrlListenerHostPort() (string, error) {
	// Get the edge controller port to use as the default
	edgeCtrlPort, err := GetZitiEdgeCtrlAdvertisedPort()
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+constants.ZitiEdgeCtrlAdvertisedPortVarName)
		if err != nil {
			return "", err
		}
	}

	return getValueOrSetAndGetDefault(constants.ZitiEdgeCtrlListenerHostPortVarName, constants.DefaultZitiEdgeListenerHost+":"+edgeCtrlPort)
}

func GetZitiEdgeCtrlAdvertisedHostPort() (string, error) {

	port, err := GetZitiEdgeCtrlAdvertisedPort()
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+constants.ZitiEdgeCtrlAdvertisedPortVarName)
		if err != nil {
			return "", err
		}
	}

	// Use External DNS if set
	extDNS := os.Getenv(constants.ExternalDNSVarName)
	if extDNS != "" {
		return extDNS + ":" + port, nil
	}

	// Use hostname and advertised port if advertised host port env var not set
	hostname, err := os.Hostname()
	if err != nil {
		err := errors.Wrap(err, "Unable to get hostname")
		if err != nil {
			return "", err
		}
	}

	return getValueOrSetAndGetDefault(constants.ZitiEdgeCtrlAdvertisedHostPortVarName, hostname+":"+port)
}

func GetZitiEdgeCtrlAdvertisedPort() (string, error) {
	return getValueOrSetAndGetDefault(constants.ZitiEdgeCtrlAdvertisedPortVarName, constants.DefaultZitiEdgeAPIPort)
}

func GetZitiEdgeIdentityEnrollmentDuration() (time.Duration, error) {
	retVal, err := getValueOrSetAndGetDefault(constants.ZitiEdgeIdentityEnrollmentDurationVarName, strconv.FormatInt(int64(edge.DefaultEdgeEnrollmentDuration.Minutes()), 10))
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+constants.ZitiEdgeIdentityEnrollmentDurationVarDescription)
		if err != nil {
			return edge.DefaultEdgeEnrollmentDuration, err
		}
	}
	retValInt, err := strconv.Atoi(retVal)
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+constants.ZitiEdgeIdentityEnrollmentDurationVarDescription)
		if err != nil {
			return edge.DefaultEdgeEnrollmentDuration, err
		}
	}

	return time.Duration(retValInt) * time.Minute, nil
}

func GetZitiEdgeRouterEnrollmentDuration() (time.Duration, error) {
	retVal, err := getValueOrSetAndGetDefault(constants.ZitiEdgeRouterEnrollmentDurationVarName, strconv.FormatInt(int64(edge.DefaultEdgeEnrollmentDuration.Minutes()), 10))
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+constants.ZitiEdgeRouterEnrollmentDurationVarDescription)
		if err != nil {
			return edge.DefaultEdgeEnrollmentDuration, err
		}
	}
	retValInt, err := strconv.Atoi(retVal)
	if err != nil {
		err := errors.Wrap(err, "Unable to get "+constants.ZitiEdgeRouterEnrollmentDurationVarDescription)
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
