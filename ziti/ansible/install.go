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

package ansible

import (
	"fmt"
	"os"
	"path/filepath"
)

// Options for installing ansible
type InstallOptions struct {
	RequirementsTXT string
	VirtualEnv      string
	Path            string
}

func InstallViaPip(options *InstallOptions) error {
	// TODO
	var (
		virtualenvBinary string
		virtualEnv       string
		pipBinary        string
		requirementsTXT  string
		err              error
	)

	if options.Path != "" {
		if _, err = os.Stat(options.Path); os.IsNotExist(err) {
			return fmt.Errorf("The specified Path (%s) does not exist", options.Path)
		}
	}

	cwd, _ := os.Getwd()

	// if no requirements.txt is set we should check to see if we have one in
	// dir in provided path or the current working dir, if so we can use that.
	if options.RequirementsTXT == "" {
		if options.Path != "" {
			requirementsTXT = filepath.Join(options.Path, "requirements.txt")
		} else {
			requirementsTXT = filepath.Join(cwd, "requirements.txt")
		}
		if requirementsTXT != "" {
			fmt.Printf("checking if %s exists... ", requirementsTXT)
			if _, err = os.Stat(requirementsTXT); err == nil {
				options.RequirementsTXT = requirementsTXT
				fmt.Println("yes")
			} else {
				fmt.Println("no")
			}
		}
	} else {
		fmt.Printf("checking if %s exists... ", options.RequirementsTXT)
		if _, err = os.Stat(options.RequirementsTXT); err == nil {
			fmt.Println("yes")
		} else {
			fmt.Println("no")
			return fmt.Errorf("(%s) does not exist", options.RequirementsTXT)
		}
	}

	// if requesting virtualenv we need to make sure virtualenv binary exists
	if options.VirtualEnv != "" {
		virtualenvBinary = checkBinInPath("virtualenv")
		if virtualenvBinary == "" {
			return fmt.Errorf("unable to find virtualenv in path")
		}
	}

	// if no virtualenv is set we should check to see if we have a virtualenv
	// dir in provided path or the current working dir, if so we can use that.
	if options.VirtualEnv == "" {
		//fmt.Printf("you specified path %s\n", options.Path)
		if options.Path != "" {
			virtualEnv = filepath.Join(options.Path, "virtualenv")
			options.VirtualEnv = virtualEnv
		} else {
			virtualEnv = filepath.Join(cwd, "virtualenv")
			fmt.Printf("checking if %s exists... ", virtualEnv)
			if _, err = os.Stat(virtualEnv); err == nil {
				options.VirtualEnv = virtualEnv
				fmt.Println("yes")
			} else {
				fmt.Println("no")
			}
		}
	}

	// if requesting virtualenv we need to make sure virtualenv binary exists
	if options.VirtualEnv != "" {
		virtualenvBinary = checkBinInPath("virtualenv")
		if virtualenvBinary == "" {
			return fmt.Errorf("unable to find virtualenv in path")
		}
	}

	// if a virtualenv is specified make sure that pip is installed
	// in it and use that, otherwise make sure its in path.
	if options.VirtualEnv != "" {
		fmt.Printf("Using VirtualEnv: %s\n", options.VirtualEnv)
		pipBinary = filepath.Join(options.VirtualEnv, "bin", "pip")
		if _, err = os.Stat(pipBinary); os.IsNotExist(err) {
			err = runCmd(virtualenvBinary, []string{options.VirtualEnv})
			if err != nil {
				return err
			}
		}
	} else {
		pipBinary = checkBinInPath("pip")
	}
	if pipBinary == "" {
		return fmt.Errorf("unable to find pip in path")
	}
	fmt.Printf("Using Pip: %s\n", pipBinary)

	if options.RequirementsTXT != "" {
		err = runCmd(pipBinary, []string{"install", "-r", options.RequirementsTXT})
		if err != nil {
			return err
		}
	} else {
		err = runCmd(pipBinary, []string{"install", "ansible"})
		if err != nil {
			return err
		}
	}

	return nil
}
