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

	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/templates"
	"github.com/spf13/cobra"
)

// UnInstallOptions are the flags for delete commands
type UnInstallOptions struct {
	CommonOptions
}

var (
	uninstall_long = templates.LongDesc(`
		UnInstall the Ziti platform binaries.
`)

	uninstall_example = templates.Examples(`
		# uninstall the Ziti router
		ziti uninstall ziti-router
	`)
)

// NewCmdUnInstall creates the command
func NewCmdUnInstall(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &UnInstallOptions{
		CommonOptions{
			Out: out,
			Err: errOut,
		},
	}

	cmd := &cobra.Command{
		Use:     "uninstall [flags]",
		Short:   "Un-Installs a Ziti component/app",
		Long:    uninstall_long,
		Example: uninstall_example,
		Aliases: []string{"uninstall"},
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{"up"},
	}

	cmd.AddCommand(NewCmdUnInstallZitiController(out, errOut))
	cmd.AddCommand(NewCmdUnInstallZitiRouter(out, errOut))
	cmd.AddCommand(NewCmdUnInstallZitiTunnel(out, errOut))
	cmd.AddCommand(NewCmdUnInstallZitiEdgeTunnel(out, errOut))
	cmd.AddCommand(NewCmdUnInstallZitiProxC(out, errOut))

	return cmd
}

// Run implements this command
func (o *UnInstallOptions) Run() error {
	return o.Cmd.Help()
}
