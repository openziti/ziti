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

package cmd

import (
	"io"

	"github.com/openziti/ziti/ziti/ansible"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/templates"
	"github.com/spf13/cobra"
)

var (
	installAnsibleLong = templates.LongDesc(`
	'ziti install ansible' will install ansible and/or any dependencies provided.
	If you do not specify a virtualenv or requirements file it will attempt
	to find them in your local path, or it will try to find them in your
	environment if you provide one.
	`)

	installAnsibleExample = templates.Examples(`
		# Install Ansible 
		ziti install ansible
	`)
)

// InstallAnsibleOptions the options for the install command
type InstallAnsibleOptions struct {
	InstallOptions

	Version string
}

var installOptions = &ansible.InstallOptions{}

// NewCmdInstallAnsible defines the command
func NewCmdInstallAnsible(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &InstallAnsibleOptions{
		InstallOptions: InstallOptions{
			CommonOptions: CommonOptions{
				Out: out,
				Err: errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:     "ansible",
		Short:   "Install ansible via pip",
		Aliases: []string{},
		Long:    installAnsibleLong,
		Example: installAnsibleExample,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}

	// stops parsing flags after first unknown flag is found
	cmd.Flags().SetInterspersed(false)

	cmd.Flags().StringVarP(&installOptions.VirtualEnv, "virtualenv",
		"v", "", "Path to VirtualEnv to use")

	cmd.Flags().StringVarP(&installOptions.RequirementsTXT, "requirements",
		"r", "", "path to requirements.txt")

	cmd.Flags().StringVarP(&installOptions.Path, "environment",
		"e", "", "path to your ziti environment")

	return cmd
}

// Run implements the command
func (o *InstallAnsibleOptions) Run() error {
	return ansible.InstallViaPip(installOptions)
}
