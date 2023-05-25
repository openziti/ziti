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
	"github.com/blang/semver"
	"github.com/openziti/ziti/common/getziti"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/templates"
	c "github.com/openziti/ziti/ziti/constants"
	"github.com/openziti/ziti/ziti/internal/log"
	"github.com/spf13/cobra"
	"io"
	"strings"
)

var (
	installZitiEdgeTunnelLong = templates.LongDesc(`
		Installs the Ziti Edge Tunnel app if it has not been installed already
`)

	installZitiEdgeTunnelExample = templates.Examples(`
		# Install the Ziti Edge Tunnel app 
		ziti install ziti-edge-tunnel
	`)
)

// InstallZitiEdgeTunnelOptions the options for the upgrade ziti-edge-tunnel command
type InstallZitiEdgeTunnelOptions struct {
	InstallOptions

	Version string
}

// NewCmdInstallZitiEdgeTunnel defines the command
func NewCmdInstallZitiEdgeTunnel(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &InstallZitiEdgeTunnelOptions{
		InstallOptions: InstallOptions{
			CommonOptions: common.CommonOptions{
				Out: out,
				Err: errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:     "ziti-edge-tunnel",
		Short:   "Installs the Ziti Edge Tunnel app - if it has not been installed already",
		Aliases: []string{"edge-tunnel"},
		Long:    installZitiEdgeTunnelLong,
		Example: installZitiEdgeTunnelExample,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}
	cmd.Flags().StringVarP(&options.Version, "version", "v", "", "The specific version to install")
	options.AddCommonFlags(cmd)
	return cmd
}

// Run implements the command
func (o *InstallOptions) installZitiEdgeTunnel(targetVersion string) error {
	var newVersion semver.Version

	if targetVersion != "" {
		newVersion = semver.MustParse(strings.TrimPrefix(targetVersion, "v"))
	} else {
		v, err := getziti.GetLatestGitHubReleaseVersion(c.ZITI_EDGE_TUNNEL_GITHUB, o.Verbose)
		if err != nil {
			return err
		}
		newVersion = v
	}

	log.Infoln("Attempting to install '" + c.ZITI_EDGE_TUNNEL + "' version: " + newVersion.String())
	return o.FindVersionAndInstallGitHubRelease(false, c.ZITI_EDGE_TUNNEL, c.ZITI_EDGE_TUNNEL_GITHUB, newVersion.String())
}

// Run implements the command
func (o *InstallZitiEdgeTunnelOptions) Run() error {
	return o.installZitiEdgeTunnel(o.Version)
}
