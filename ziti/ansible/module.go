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
)

// ModuleOptions for running ansible
type ModuleOptions struct {
	ModuleHosts string
	Module      string
	ModulePath  string
	ModuleArgs  string
}

// Module ansible playbook
func Module(options *Options, ansibleArgs []string) error {
	var (
		err      error
		zitiArgs []string
	)

	err = configureEnvironment(options)
	if err != nil {
		return err
	}
	err = sshConfigFile(options)
	if err != nil {
		return err
	}
	err = configureKnownHostsFile(options)
	if err != nil {
		return err
	}
	configureSSHForwardAgent(options)

	zitiArgs = []string{options.ModuleHosts, "--module-name", options.Module}

	if options.ModulePath != "" {
		zitiArgs = append(zitiArgs,
			[]string{"--module-path", options.ModulePath}...)
	}

	if options.ModuleArgs != "" {
		zitiArgs = append(zitiArgs,
			[]string{"--args", fmt.Sprintf("'%s'", options.ModuleArgs)}...)
	}

	if options.Inventory != "" {
		zitiArgs = append(zitiArgs,
			[]string{"--inventory", options.Inventory}...)
	}

	cmdName := "ansible"
	cmdArgs := append(zitiArgs, ansibleArgs...)

	return runCmd(cmdName, cmdArgs)

}
