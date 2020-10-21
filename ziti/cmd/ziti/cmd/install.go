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
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/templates"
	"github.com/spf13/cobra"
)

// InstallOptions are the flags for delete commands
type InstallOptions struct {
	CommonOptions
}

var (
	install_long = templates.LongDesc(`
		Install the Ziti platform binaries.
`)

	install_example = templates.Examples(`
		# install the Ziti router
		ziti install ziti-router
	`)
)

// NewCmdInstall creates the command
func NewCmdInstall(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &InstallOptions{
		CommonOptions{
			Factory: f,
			Out:     out,
			Err:     errOut,
		},
	}

	cmd := &cobra.Command{
		Use:     "install [flags]",
		Short:   "Installs a Ziti component/app",
		Long:    install_long,
		Example: install_example,
		Aliases: []string{"install"},
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{"up"},
	}

	cmd.AddCommand(NewCmdInstallZitiALL(f, out, errOut))

	cmd.AddCommand(NewCmdInstallZitiController(f, out, errOut))
	cmd.AddCommand(NewCmdInstallZitiFabric(f, out, errOut))
	cmd.AddCommand(NewCmdInstallZitiFabricTest(f, out, errOut))
	cmd.AddCommand(NewCmdInstallZitiMgmtGw(f, out, errOut))
	cmd.AddCommand(NewCmdInstallZitiRouter(f, out, errOut))
	cmd.AddCommand(NewCmdInstallZitiTunnel(f, out, errOut))
	cmd.AddCommand(NewCmdInstallZitiEdgeTunnel(f, out, errOut))
	cmd.AddCommand(NewCmdInstallZitiEnroller(f, out, errOut))
	cmd.AddCommand(NewCmdInstallZitiProxy(f, out, errOut))
	cmd.AddCommand(NewCmdInstallZitiProxC(f, out, errOut))

	// cmd.AddCommand(NewCmdInstallAnsible(f, out, errOut))		// Disable/hide this for now

	cmd.AddCommand(NewCmdInstallTerraformProviderEdgeController(f, out, errOut))

	options.addCommonFlags(cmd)

	return cmd
}

// Run implements this command
func (o *InstallOptions) Run() error {
	return o.Cmd.Help()
}
