/*
	Copyright NetFoundry, Inc.

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

	cmdutil "github.com/openziti/ziti/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/spf13/cobra"
)

// CreateConfigOptions the options for the create spring command
type CreateConfigOptions struct {
	CreateOptions

	Namespace   string
	Version     string
	ReleaseName string
	HelmUpdate  bool
}

// NewCmdCreateConfig creates a command object for the "create" command
func NewCmdCreateConfig(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &CreateConfigOptions{
		CreateOptions: CreateOptions{
			CommonOptions: CommonOptions{
				Factory: f,
				Out:     out,
				Err:     errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:     "config",
		Short:   "Creates a config file for specified Ziti component",
		Aliases: []string{"cfg"},
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}

	cmd.AddCommand(NewCmdCreateConfigController(f, out, errOut))
	cmd.AddCommand(NewCmdCreateConfigRouter(f, out, errOut))
	cmd.AddCommand(NewCmdCreateConfigMgmt(f, out, errOut))
	cmd.AddCommand(NewCmdCreateConfigGw(f, out, errOut))

	options.addFlags(cmd, "", "")
	return cmd
}

// Add flags that are global to all "create config" commands
func (options *CreateConfigOptions) addFlags(cmd *cobra.Command, defaultNamespace string, defaultOptionRelease string) {
}

// Run implements this command
func (options *CreateConfigOptions) Run() error {
	return options.Cmd.Help()
}
