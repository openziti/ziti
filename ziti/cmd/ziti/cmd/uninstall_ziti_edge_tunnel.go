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

	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/templates"
	c "github.com/openziti/ziti/ziti/cmd/ziti/constants"
	"github.com/spf13/cobra"
)

var (
	uninstallZitiEdgeTunnelLong = templates.LongDesc(`
		UnInstalls the Ziti Edge Tunnel app if it has previously been installed
`)

	uninstallZitiEdgeTunnelExample = templates.Examples(`
		# UnInstall the Ziti Edge Tunnel app 
		ziti uninstall ziti-edge-tunnel
	`)
)

// UnInstallZitiEdgeTunnelOptions the options for the upgrade ziti-edge-tunnel command
type UnInstallZitiEdgeTunnelOptions struct {
	UnInstallOptions

	Version string
}

// NewCmdUnInstallZitiEdgeTunnel defines the command
func NewCmdUnInstallZitiEdgeTunnel(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &UnInstallZitiEdgeTunnelOptions{
		UnInstallOptions: UnInstallOptions{
			CommonOptions: CommonOptions{
				Out: out,
				Err: errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:     "ziti-edge-tunnel",
		Short:   "UnInstalls the Ziti Edge Tunnel app - if it has previously been installed",
		Aliases: []string{"edge-tunnel"},
		Long:    uninstallZitiEdgeTunnelLong,
		Example: uninstallZitiEdgeTunnelExample,
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
func (o *UnInstallZitiEdgeTunnelOptions) Run() error {
	o.deleteInstalledBinary(c.ZITI_EDGE_TUNNEL)
	return nil
}
