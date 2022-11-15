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
	upgradeZitiProxCLong = templates.LongDesc(`
		Upgrades the Ziti ProxC app if there is a newer release
`)

	upgradeZitiProxCExample = templates.Examples(`
		# Upgrades the Ziti ProxC app 
		ziti upgrade ziti-prox-c
	`)
)

// UpgradeZitiProxCOptions the options for the upgrade ziti-prox-c command
type UpgradeZitiProxCOptions struct {
	InstallOptions

	Version string
}

// NewCmdUpgradeZitiProxC defines the command
func NewCmdUpgradeZitiProxC(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &UpgradeZitiProxCOptions{
		InstallOptions: InstallOptions{
			CommonOptions: common.CommonOptions{
				Out: out,
				Err: errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:     "ziti-prox-c",
		Short:   "Upgrades the Ziti ProxC app - if there is a new version available",
		Aliases: []string{"proxc"},
		Long:    upgradeZitiProxCLong,
		Example: upgradeZitiProxCExample,
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
func (o *UpgradeZitiProxCOptions) Run() error {
	newVersion, err := o.getLatestGitHubReleaseVersion(c.ZITI_SDK_C_GITHUB)
	if err != nil {
		return err
	}

	newVersionStr := newVersion.String()

	if o.Version != "" {
		newVersionStr = o.Version
	}

	o.deleteInstalledBinary(c.ZITI_PROX_C)

	return o.findVersionAndInstallGitHubRelease(c.ZITI_PROX_C, c.ZITI_SDK_C_GITHUB, true, newVersionStr)
}
