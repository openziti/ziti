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
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/templates"
	c "github.com/openziti/ziti/ziti/constants"
	"github.com/openziti/ziti/ziti/internal/log"
	"io"

	"github.com/blang/semver"
	"github.com/openziti/ziti/common/version"
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
func NewCmdInstallZitiTunnel(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &InstallZitiTunnelOptions{
		InstallOptions: InstallOptions{
			CommonOptions: common.CommonOptions{
				Out: out,
				Err: errOut,
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
	options.AddCommonFlags(cmd)
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
