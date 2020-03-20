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

	cmdutil "github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/helpers"
	"github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/templates"
	c "github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/constants"
	"github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/internal/log"
	"github.com/netfoundry/ziti-cmd/common/version"
	"github.com/blang/semver"
	"github.com/spf13/cobra"
)

var (
	installZitiTunnelLong = templates.LongDesc(`
		Installs the Ziti Tunnel app if it has not been installed already
`)

	installZitiTunnelExample = templates.Examples(`
		# Install the Ziti Tunnel app 
		ziti install ziti-tunnel
	`)
)

// InstallZitiTunnelOptions the options for the upgrade ziti-tunnel command
type InstallZitiTunnelOptions struct {
	InstallOptions

	Version string
}

// NewCmdInstallZitiTunnel defines the command
func NewCmdInstallZitiTunnel(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &InstallZitiTunnelOptions{
		InstallOptions: InstallOptions{
			CommonOptions: CommonOptions{
				Factory: f,
				Out:     out,
				Err:     errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:     "ziti-tunnel",
		Short:   "Installs the Ziti Tunnel app - if it has not been installed already",
		Aliases: []string{"tunnel"},
		Long:    installZitiTunnelLong,
		Example: installZitiTunnelExample,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}
	cmd.Flags().StringVarP(&options.Version, "version", "v", "", "The specific version to install")
	options.addCommonFlags(cmd)
	return cmd
}

// Run implements the command
func (o *InstallZitiTunnelOptions) Run() error {
	newVersion, err := o.getLatestZitiAppVersion(version.GetBranch(), c.ZITI_TUNNEL)
	if err != nil {
		return err
	}

	if o.Version != "" {
		newVersion, err = semver.Make(o.Version)
	}

	log.Infoln("Attempting to install '" + c.ZITI_TUNNEL + "' version: " + newVersion.String())

	return o.installZitiApp(version.GetBranch(), c.ZITI_TUNNEL, false, newVersion.String())
}
