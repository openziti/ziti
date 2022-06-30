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
	"os"
	"os/exec"
	"path/filepath"

	"github.com/openziti/ziti/ziti/ansible"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	// "github.com/openziti/ziti/ziti/cmd/ziti/cmd/templates"
	"github.com/spf13/cobra"
)

// PingOptions are the flags for delete commands
type PingOptions struct {
	CommonOptions
}

var pingOptions = &ansible.Options{}

// NewCmdPing creates the command
func NewCmdPing(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &PingOptions{
		CommonOptions{
			Out: out,
			Err: errOut,
		},
	}

	cmd := &cobra.Command{
		Use:   "ping [flags] [--] [ansible arguments]",
		Short: "check if all hosts are available",
		Long: `
'ziti' ping' uses the ansible ping module to check if all hosts are available.
This is useful for checking if your machine can successfully SSH/Ansible to
each host.
`,
		Aliases: []string{},
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
			if pingOptions.Environment != cwd {
				virtualEnv = filepath.Join(pingOptions.Environment, "virtualenv/bin")
				if _, err = os.Stat(virtualEnv); err == nil {
					os.Setenv("PATH", fmt.Sprintf("%s:%s", virtualEnv, os.Getenv("PATH")))
				}
			}
			// check if ansible-playbook binary exists
			_, err = exec.LookPath("ansible")
			if err != nil {
				return err
			}
			return nil
		},

		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// stops parsing flags after first unknown flag is found
	cmd.Flags().SetInterspersed(false)

	cmd.Flags().StringVarP(&pingOptions.SSHConfigFile, "ssh-config-file",
		"s", "", "Path to ssh config file to use.")

	cmd.Flags().StringVarP(&pingOptions.Environment, "environment",
		"e", "", "directory that contains ansible inventory")

	cmd.Flags().StringVarP(&pingOptions.KnownHostsFile, "known-hosts-file",
		"", "", "location of known hosts file")

	cmd.Flags().StringVarP(&pingOptions.ModuleHosts, "hosts",
		"", "all", "host or host pattern to run")

	return cmd
}

// Run implements this command
func (o *PingOptions) Run() error {
	pingOptions.Module = "ping"

	// if len(o.Args) < 1 {
	// 	return o.Cmd.Help()
	// }

	return ansible.Module(pingOptions, o.Args)
}
