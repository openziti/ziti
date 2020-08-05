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
	c "github.com/openziti/ziti/ziti/cmd/ziti/constants"
	"github.com/openziti/ziti/common/version"
	"github.com/spf13/cobra"
)

var (
	upgradeZitiProxyLong = templates.LongDesc(`
		Upgrades the Ziti Proxy app if there is a newer release
`)

	upgradeZitiProxyExample = templates.Examples(`
		# Upgrades the Ziti Proxy app 
		ziti upgrade ziti-proxy
	`)
)

// UpgradeZitiProxyOptions the options for the upgrade ziti-proxy command
type UpgradeZitiProxyOptions struct {
	CreateOptions

	Version string
}

// NewCmdUpgradeZitiProxy defines the command
func NewCmdUpgradeZitiProxy(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &UpgradeZitiProxyOptions{
		CreateOptions: CreateOptions{
			CommonOptions: CommonOptions{
				Factory: f,
				Out:     out,
				Err:     errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:     "ziti-proxy",
		Short:   "Upgrades the Ziti Proxy app - if there is a new version available",
		Aliases: []string{"proxy"},
		Long:    upgradeZitiProxyLong,
		Example: upgradeZitiProxyExample,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}
	cmd.Flags().StringVarP(&options.Version, "version", "v", "", "The specific version to upgrade to")
	options.addCommonFlags(cmd)
	return cmd
}

// Run implements the command
func (o *UpgradeZitiProxyOptions) Run() error {
	newVersion, err := o.getLatestZitiAppVersion(version.GetBranch(), c.ZITI_PROXY)
	if err != nil {
		return err
	}

	newVersionStr := newVersion.String()

	if o.Version != "" {
		newVersionStr = o.Version
	}

	o.deleteInstalledBinary(c.ZITI_PROXY)

	return o.installZitiApp(version.GetBranch(), c.ZITI_PROXY, true, newVersionStr)
}
