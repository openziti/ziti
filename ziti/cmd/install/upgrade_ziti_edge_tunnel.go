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

package install

import (
	"github.com/openziti/ziti/ziti/cmd/common"
	"io"

	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/templates"
	c "github.com/openziti/ziti/ziti/constants"
	"github.com/spf13/cobra"
)

var (
	upgradeZitiEdgeTunnelLong = templates.LongDesc(`
		Upgrades the Ziti Edge Tunnel app if there is a newer release
`)

	upgradeZitiEdgeTunnelExample = templates.Examples(`
		# Upgrades the Ziti Edge Tunnel app 
		ziti upgrade ziti-edge-tunnel
	`)
)

// UpgradeZitiEdgeTunnelOptions the options for the upgrade ziti-edge-tunnel command
type UpgradeZitiEdgeTunnelOptions struct {
	InstallOptions

	Version string
}

// NewCmdUpgradeZitiEdgeTunnel defines the command
func NewCmdUpgradeZitiEdgeTunnel(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &UpgradeZitiEdgeTunnelOptions{
		InstallOptions: InstallOptions{
			CommonOptions: common.CommonOptions{
				Out: out,
				Err: errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:     "ziti-edge-tunnel",
		Short:   "Upgrades the Ziti Edge Tunnel app - if there is a new version available",
		Aliases: []string{"edge-tunnel", "et"},
		Long:    upgradeZitiEdgeTunnelLong,
		Example: upgradeZitiEdgeTunnelExample,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}
	cmd.Flags().StringVarP(&options.Version, "version", "v", "", "The specific version to upgrade to")
	options.AddCommonFlags(cmd)
	return cmd
}

// Run implements the command
func (o *UpgradeZitiEdgeTunnelOptions) Run() error {
	newVersion, err := o.getLatestGitHubReleaseVersion(c.ZITI_EDGE_TUNNEL_GITHUB)
	if err != nil {
		return err
	}

	newVersionStr := newVersion.String()

	if o.Version != "" {
		newVersionStr = o.Version
	}

	o.deleteInstalledBinary(c.ZITI_EDGE_TUNNEL)

	return o.findVersionAndInstallGitHubRelease(c.ZITI_EDGE_TUNNEL, c.ZITI_EDGE_TUNNEL_GITHUB, true, newVersionStr)
}
