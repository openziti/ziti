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

	"github.com/openziti/ziti/common/version"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/templates"
	c "github.com/openziti/ziti/ziti/constants"
	"github.com/spf13/cobra"
)

var (
	upgradeZitiRouterLong = templates.LongDesc(`
		Upgrades the Ziti Router app if there is a newer release
`)

	upgradeZitiRouterExample = templates.Examples(`
		# Upgrades the Ziti Router app 
		ziti upgrade ziti-router
	`)
)

// UpgradeZitiRouterOptions the options for the upgrade ziti-router command
type UpgradeZitiRouterOptions struct {
	InstallOptions

	Version string
}

// NewCmdUpgradeZitiRouter defines the command
func NewCmdUpgradeZitiRouter(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &UpgradeZitiRouterOptions{
		InstallOptions: InstallOptions{
			CommonOptions: common.CommonOptions{
				Out: out,
				Err: errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:     "ziti-router",
		Short:   "Upgrades the Ziti Router app - if there is a new version available",
		Aliases: []string{"router", "rtr", "r"},
		Long:    upgradeZitiRouterLong,
		Example: upgradeZitiRouterExample,
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
func (o *UpgradeZitiRouterOptions) Run() error {
	newVersion, err := o.getLatestZitiAppVersion(version.GetBranch(), c.ZITI_ROUTER)
	if err != nil {
		return err
	}

	newVersionStr := newVersion.String()

	if o.Version != "" {
		newVersionStr = o.Version
	}

	o.deleteInstalledBinary(c.ZITI_ROUTER)

	return o.installZitiApp(version.GetBranch(), c.ZITI_ROUTER, true, newVersionStr)
}
