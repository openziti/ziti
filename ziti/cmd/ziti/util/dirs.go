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

package util

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
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

func ConfigDir() (string, error) {
	path := os.Getenv("ZITI_HOME")
	if path != "" {
		return path, nil
	}
	h := HomeDir()
	if runtime.GOOS == "linux" {
		path = filepath.Join(h, ".config/ziti")
	} else {
		path = filepath.Join(h, ".ziti")
	}

	err := os.MkdirAll(path, DefaultWritePermissions)
	if err != nil {
		return "", err
	}
	return path, nil
}

func FabricConfigDir() (string, error) {
	h, err := ConfigDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(h, "fabric")
	err = os.MkdirAll(path, DefaultWritePermissions)
	if err != nil {
		return "", err
	}
	return path, nil
}

func ZitiAppConfigDir(zitiApp string) (string, error) {
	h, err := ConfigDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(h, zitiApp)
	err = os.MkdirAll(path, DefaultWritePermissions)
	if err != nil {
		return "", err
	}
	return path, nil
}

func CacheDir() (string, error) {
	h, err := ConfigDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(h, "cache")
	err = os.MkdirAll(path, DefaultWritePermissions)
	if err != nil {
		return "", err
	}
	return path, nil
}

func EnvironmentsDir() (string, error) {
	h, err := ConfigDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(h, "environments")
	err = os.MkdirAll(path, DefaultWritePermissions)
	if err != nil {
		return "", err
	}
	return path, nil
}

func NewEnvironmentDir(envName string) (string, error) {
	h, err := EnvironmentsDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(h, envName)

	_, err = os.Stat(path)
	if err == nil {
		return "", fmt.Errorf("Environment dir (%s) already exists", envName)
	}

	err = os.MkdirAll(path, DefaultWritePermissions)
	if err != nil {
		return "", err
	}
	return path, nil
}

func PKIRootDir() (string, error) {
	var path string
	var err error
	cwd, _ := os.Getwd()
	testInventory := filepath.Join(cwd, "hosts")
	if _, err := os.Stat(testInventory); os.IsNotExist(err) {
		h, err := EnvironmentsDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(h, "pki")
	} else {
		path = filepath.Join(cwd, "pki")
	}
	err = os.MkdirAll(path, DefaultWritePermissions)
	if err != nil {
		return "", err
	}
	return path, nil
}

func BinaryLocation() (string, error) {
	h, err := ConfigDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(h, "bin")
	err = os.MkdirAll(path, DefaultWritePermissions)
	if err != nil {
		return "", err
	}
	return path, nil
}

// TerraformProviderBinaryLocation provides the proper location to place a Terraform provider based on the currently running OS.
// In Mac/Linux, it's `~/.terraform.d/plugins` and on Windows it's `%APPDATA%\terraform.d\plugins`
func TerraformProviderBinaryLocation() (string, error) {
	var path string
	h := HomeDir()
	if runtime.GOOS == "windows" {
		h = os.Getenv("APPDATA")
		if h == "" {
			return "", fmt.Errorf("APPDATA env var missing; install of Terraform provider cannot proceed")
		}
		path = filepath.Join(h, "terraform.d/plugins")
	} else {
		path = filepath.Join(h, ".terraform.d/plugins")
	}
	err := os.MkdirAll(path, DefaultWritePermissions)
	if err != nil {
		return "", err
	}
	return path, nil
}

// WriteZitiAppConfigFile writes out the config file data for the given Ziti application to the appropriate config file
func WriteZitiAppConfigFile(zitiApp string, configData interface{}) error {
	return WriteZitiAppFile(zitiApp, "config", configData)
}

// WriteZitiAppFile writes application data (config, session, preferences, etc) to an appropriate location
func WriteZitiAppFile(zitiApp string, fileType string, appData interface{}) error {
	configDir, err := ZitiAppConfigDir(zitiApp)
	if err != nil {
		return err
	}
	filePath := filepath.Join(configDir, fileType+".json")

	data, err := json.MarshalIndent(appData, "", "    ")
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filePath, data, 0600)
}

// ReadZitiAppConfigFile reads in the config file data for the given Ziti application from an appropriate location
func ReadZitiAppConfigFile(zitiApp string, configData interface{}) error {
	return ReadZitiAppFile(zitiApp, "config", configData)
}

// ReadZitiAppFile reads application data (config, session, preferences, etc) for the given Ziti application from an appropriate location
func ReadZitiAppFile(zitiApp string, fileType string, configData interface{}) error {
	configDir, err := ZitiAppConfigDir(zitiApp)
	if err != nil {
		return err
	}

	filePath := filepath.Join(configDir, fileType+".json")

	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return err
	}

	err = json.Unmarshal(data, configData)

	return err
}
