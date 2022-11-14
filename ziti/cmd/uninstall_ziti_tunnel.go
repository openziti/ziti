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
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/templates"
	c "github.com/openziti/ziti/ziti/constants"
	"io"

	"github.com/spf13/cobra"
)

var (
	uninstallZitiTunnelLong = templates.LongDesc(`
		UnInstalls the Ziti Tunnel app if it has previously been installed
`)

	uninstallZitiTunnelExample = templates.Examples(`
		# UnInstall the Ziti Tunnel app 
		ziti uninstall ziti-tunnel
	`)
)

// UnInstallZitiTunnelOptions the options for the upgrade ziti-tunnel command
type UnInstallZitiTunnelOptions struct {
	UnInstallOptions

	Version string
}

// NewCmdUnInstallZitiTunnel defines the command
func NewCmdUnInstallZitiTunnel(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &UnInstallZitiTunnelOptions{
		UnInstallOptions: UnInstallOptions{
			CommonOptions: CommonOptions{
				Out: out,
				Err: errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:     "ziti-tunnel",
		Short:   "UnInstalls the Ziti Tunnel app - if it has previously been installed",
		Aliases: []string{"tunnel"},
		Long:    uninstallZitiTunnelLong,
		Example: uninstallZitiTunnelExample,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}
	return cmd
}

// Run implements the command
func (o *UnInstallZitiTunnelOptions) Run() error {
	o.deleteInstalledBinary(c.ZITI_TUNNEL)
	return nil
}
