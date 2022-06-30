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

package provisioner

import (
	"github.com/openziti/ziti/ziti/vagrantutil"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

var vagrant *vagrantutil.Vagrant

func findVagrantfile(paths []string) string {
	var (
		vagrantFile string
		path        string
	)
	for _, path = range paths {
		vagrantFile = filepath.Join(path, "Vagrantfile")
		if _, err := os.Stat(vagrantFile); !os.IsNotExist(err) {
			return path
		}
	}
	return ""
}

func runVagrantUp() {
	output, _ := vagrant.Up()
	// print the output
	for line := range output {
		fmt.Println(line)
	}
}

// VagrantUp provisioner
func VagrantUp(po *Options) error {
	var (
		err error
	)
	cwd, _ := os.Getwd()
	paths := []string{po.Environment, cwd}
	vagrantPath := findVagrantfile(paths)
	if vagrantPath == "" {
		return fmt.Errorf("cannot find vagrantfile in %s", paths)
	}
	vagrant, _ = vagrantutil.NewVagrant(vagrantPath)
	vagrant.Status()
	// fmt.Println("vagrant.State: %s", vagrant.State)
	switch state := vagrant.State; state {
	case "NotCreated":
		runVagrantUp()
	case "PowerOff":
		runVagrantUp()
	case "Saved":
		runVagrantUp()
	case "Running":
		// do nothing
	default:
		return fmt.Errorf("cannot handle vagrant state %s.", state)
	}

	if vagrant.State == "NotCreated" {
		output, _ := vagrant.Up()
		// print the output
		for line := range output {
			fmt.Println(line)
		}
	}
	sshConfig, err := (vagrant.SSHConfig())
	if err != nil {
		return err
	}
	sshConfigFile := filepath.Join(vagrantPath, "ssh_config")
	err = ioutil.WriteFile(sshConfigFile, []byte(sshConfig), 0644)
	if err != nil {
		return err
	}

	return nil
}
