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
	"fmt"
	"io"

	"github.com/openziti/ziti/ziti/ansible"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/provisioner"

	"github.com/spf13/cobra"
	"os"
	"os/exec"
	"path/filepath"
)

const ()

type PlaybookOptions struct {
	CommonOptions
}

var playbookOptions = &ansible.Options{}
var po = &provisioner.Options{}

func NewCmdPlaybook(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &PlaybookOptions{
		CommonOptions: CommonOptions{
			Out: out,
			Err: errOut,
		},
	}

	cmd := &cobra.Command{
		Use:   "playbook [flags] file.yml [--] [ansible-playbook arguments]",
		Short: "Wrapper around ansible command",
		Long: `
'ziti playbook' will run an ansible playbook over your provided environment.

You can set additional ansible-playbook flags by providing a double dash "--" and 
then the additional flags.

Example:

To run playbook/yaml over the env/test environment passing flags to ansible and use 
the user "root" and run in verbose mode, do this:

	$ ziti playbook -e env/test playbook.yml -- --user=root -vvvv
`,
		Aliases: []string{"up"},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			var (
				err        error
				virtualEnv string
			)

			// check if there's a virtualenv we should use in your cwd
			cwd, _ := os.Getwd()
			virtualEnv = filepath.Join(cwd, "virtualenv/bin")
			if _, err = os.Stat(virtualEnv); err == nil {
				os.Setenv("PATH", fmt.Sprintf("%s:%s", virtualEnv, os.Getenv("PATH")))
			}

			// check if there's a virtualenv we should use in your environment
			if playbookOptions.Environment != cwd {
				virtualEnv = filepath.Join(playbookOptions.Environment, "virtualenv/bin")
				if _, err = os.Stat(virtualEnv); err == nil {
					os.Setenv("PATH", fmt.Sprintf("%s:%s", virtualEnv, os.Getenv("PATH")))
				}
			}

			// check if ansible-playbook binary exists
			_, err = exec.LookPath("ansible-playbook")
			if err != nil {
				return err
			}
			// check if provisioning needs to happen
			if po.Provisioner != "" {
				po.Environment = playbookOptions.Environment
				err = provisioner.Up(po)
				if err != nil {
					return err
				}
			}

			return nil
		},

		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}
	options.AddCommonFlags(cmd)

	cmd.Flags().StringVarP(&playbookOptions.SSHConfigFile, "ssh-config-file",
		"s", "", "Path to ssh config file to use.")

	cmd.Flags().BoolVarP(&playbookOptions.SSHForwardAgent, "ssh-forward-agent",
		"f", false, "path to ssh config file to use")

	cmd.Flags().StringVarP(&playbookOptions.Environment, "environment",
		"e", "", "directory that contains ansible inventory")

	cmd.Flags().StringVarP(&playbookOptions.KnownHostsFile, "known-hosts-file",
		"", "", "location of known hosts file")

	cmd.Flags().StringVarP(&po.Provisioner, "provisioner",
		"", "", "provisioner (vagrant)")

	return cmd
}

// Run ...
func (o *PlaybookOptions) Run() error {
	if len(o.Args) < 1 {
		return o.Cmd.Help()
	}
	return ansible.PlaybookRun(playbookOptions, o.Args)
}
