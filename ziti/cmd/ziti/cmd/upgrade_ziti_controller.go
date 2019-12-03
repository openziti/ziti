/*
	Copyright 2019 Netfoundry, Inc.

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
	"github.com/netfoundry/ziti-cmd/common/version"
	"github.com/spf13/cobra"
)

var (
	upgradeZitiControllerLong = templates.LongDesc(`
		Upgrades the Ziti Controller app if there is a newer release
`)

	upgradeZitiControllerExample = templates.Examples(`
		# Upgrades the Ziti Controller app 
		ziti upgrade ziti-controller
	`)
)

// UpgradeZitiControllerOptions the options for the upgrade ziti-controller command
type UpgradeZitiControllerOptions struct {
	CreateOptions

	Version string
}

// NewCmdUpgradeZitiController defines the command
func NewCmdUpgradeZitiController(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &UpgradeZitiControllerOptions{
		CreateOptions: CreateOptions{
			CommonOptions: CommonOptions{
				Factory: f,
				Out:     out,
				Err:     errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:     "ziti-controller",
		Short:   "Upgrades the Ziti Controller app - if there is a new version available",
		Aliases: []string{"controller", "ctrl", "zc"},
		Long:    upgradeZitiControllerLong,
		Example: upgradeZitiControllerExample,
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
func (o *UpgradeZitiControllerOptions) Run() error {
	newVersion, err := o.getLatestZitiAppVersion(version.GetBranch(), c.ZITI_CONTROLLER)
	if err != nil {
		return err
	}

	newVersionStr := newVersion.String()

	if o.Version != "" {
		newVersionStr = o.Version
	}

	o.deleteInstalledBinary(c.ZITI_CONTROLLER)

	return o.installZitiApp(version.GetBranch(), c.ZITI_CONTROLLER, true, newVersionStr)
}
