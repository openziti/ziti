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
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Options for running ansible
type Options struct {
	SSHConfigFile   string
	SSHForwardAgent bool
	Provisioner     string
	Environment     string
	Inventory       string
	KnownHostsFile  string
	ModuleOptions
	PlaybookOptions
}

// sets an environment variable
func setEnvironmentVariables(variable, value string) {
	os.Setenv(variable, value)
}

// appends a string to an existing environment variable
func appendEnvironmentVariable(variable, value string) {
	old := os.Getenv(variable)
	new := []string{old, value}
	os.Setenv(variable, strings.Join(new, " "))
}

// Checks if specified ssh config file exists, if not it
// checks if there is an ssh_config in the specified environment
// otherwise defaults to no ssh config.
// appends result to env var ANSIBLE_SSH_ARGS
func sshConfigFile(options *Options) error {
	if options.SSHConfigFile == "" {
		sshConfigFile := filepath.Join(options.Environment, "ssh_config")
		if _, err := os.Stat(sshConfigFile); !os.IsNotExist(err) {
			options.SSHConfigFile = sshConfigFile
		}
	} else {
		if _, err := os.Stat(options.SSHConfigFile); os.IsNotExist(err) {
			return fmt.Errorf("ssh_config file %s does not exist", options.SSHConfigFile)
		}
	}
	if options.SSHConfigFile != "" {
		appendEnvironmentVariable("ANSIBLE_SSH_ARGS",
			fmt.Sprintf("-F %s", options.SSHConfigFile))
	}
	return nil
}

// isEnvironment checks if a given directory is a suitable environment
func isEnvironment(path string) bool {
	fi, err := os.Stat(path)
	switch {
	case err != nil:
		return false
	case fi.IsDir():
		testInventory := filepath.Join(path, "hosts")
		if _, err := os.Stat(testInventory); os.IsNotExist(err) {
			return false
		}
		return true
	default:
		return false
	}
}

// if the user specifies an environment we will attempt to ensure that
// it exists, we will also set the inventory arg to be passed onto ansible.
func configureEnvironment(options *Options) error {
	if options.Environment != "" {
		if isEnvironment(options.Environment) {
			options.Inventory = filepath.Join(options.Environment, "hosts")
		} else {
			return fmt.Errorf("%s is not a valid environment path", options.Environment)
		}
	} else {
		cwd, _ := os.Getwd()
		fmt.Printf("--environment not set.  Trying your working directory (%s)\n", cwd)
		if isEnvironment(cwd) {
			options.Environment = cwd
			options.Inventory = filepath.Join(cwd, "hosts")
		} else {
			return fmt.Errorf("%s is not a valid environment path", cwd)
		}
	}

	if options.Inventory != "" {
		if _, err := os.Stat(options.Inventory); os.IsNotExist(err) {
			return fmt.Errorf("inventory file %s does not exist", options.Inventory)
		}
	}
	return nil
}

// checks if the user specifies an alternative known hosts file, if not
// looks for one in the specified environment or just defaults to none.
func configureKnownHostsFile(options *Options) error {
	if options.KnownHostsFile != "" {
		if _, err := os.Stat(options.KnownHostsFile); os.IsNotExist(err) {
			return fmt.Errorf("known hosts file %s does not exist", options.KnownHostsFile)
		}
	} else {
		maybeKnownHostsFile := filepath.Join(options.Environment, "ssh_known_hosts")
		if _, err := os.Stat(maybeKnownHostsFile); !os.IsNotExist(err) {
			options.KnownHostsFile = maybeKnownHostsFile
		}
	}
	if options.KnownHostsFile != "" {
		appendEnvironmentVariable("ANSIBLE_SSH_ARGS",
			fmt.Sprintf("-o UserKnownHostsFile=%s", options.KnownHostsFile))
	}
	return nil
}

// enables ssh agent forwarding
func configureSSHForwardAgent(options *Options) {
	if options.SSHForwardAgent {
		appendEnvironmentVariable("ANSIBLE_SSH_ARGS", "-o ForwardAgent=yes")
	}
}

func init() {
	// setEnvironmentVariables("ANSIBLE_STDOUT_CALLBACK","json")
}

func checkBinInPath(binary string) string {
	fmt.Printf("==> checkBinInPath: %s\n", binary)
	location, err := exec.LookPath(binary)
	if err != nil {
		fmt.Printf("==> checkBinInPath: err: %s\n", err)
		return ""
	}
	return location
}

// runCmd takes a command and args and runs it, streaming output to stdout
func runCmd(cmdName string, cmdArgs []string) error {
	fmt.Printf("==> Running: %s %s\n", cmdName, strings.Join(cmdArgs, " "))
	//return fmt.Errorf("bye")
	cmd := exec.Command(cmdName, cmdArgs...)
	cmdReader, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(cmdReader)
	go func() {
		for scanner.Scan() {
			fmt.Printf("%s\n", scanner.Text())
		}
	}()

	err = cmd.Start()
	if err != nil {
		return err
	}

	err = cmd.Wait()
	if err != nil {
		return err
	}
	return nil
}
