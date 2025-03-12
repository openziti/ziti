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
	path := os.Getenv("ZITI_CONFIG_DIR")
	if path != "" {
		return path, nil
	}
	h := HomeDir()
	if runtime.GOOS == "linux" {
		path = filepath.Join(h, ".config/ziti")
	} else {
		path = filepath.Join(h, ".ziti")
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
