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
	"fmt"
	"github.com/blang/semver"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/table"
	"github.com/openziti/ziti/ziti/cmd/ziti/internal/log"
	"io"
	// "strings"

	"github.com/openziti/ziti/common/version"
	cmdutil "github.com/openziti/ziti/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	c "github.com/openziti/ziti/ziti/cmd/ziti/constants"
	"github.com/openziti/ziti/ziti/cmd/ziti/util"

	"github.com/spf13/cobra"
)

const ()

type VersionOptions struct {
	CommonOptions

	Container      string
	NoVersionCheck bool
}

func NewCmdVersion(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &VersionOptions{
		CommonOptions: CommonOptions{
			Factory: f,
			Out:     out,
			Err:     errOut,
		},
	}

	cmd := &cobra.Command{
		Use:     "version",
		Short:   "Print the version information",
		Aliases: []string{"ver", "v"},
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}
	options.addCommonFlags(cmd)

	cmd.Flags().BoolVarP(&options.NoVersionCheck, "no-update", "n", false,
		"disable update check")

	return cmd
}

// Run ...
func (o *VersionOptions) Run() error {

	util.ConfigDir()

	info := util.ColorInfo

	table := o.CreateTable()
	table.AddRow("NAME", "VERSION")

	table.AddRow(c.ZITI, info(version.GetBuildMetadata(o.Verbose)))

	o.versionPrintZitiApp(c.ZITI_CONTROLLER, &table)
	o.versionPrintZitiApp(c.ZITI_ENROLLER, &table)
	o.versionPrintZitiApp(c.ZITI_FABRIC, &table)
	o.versionPrintZitiApp(c.ZITI_FABRIC_GW, &table)
	o.versionPrintZitiApp(c.ZITI_FABRIC_TEST, &table)
	o.versionPrintZitiApp(c.ZITI_PROX_C, &table)
	o.versionPrintZitiApp(c.ZITI_ROUTER, &table)
	o.versionPrintZitiApp(c.ZITI_TUNNEL, &table)
	o.versionPrintZitiApp(c.ZITI_EDGE_TUNNEL, &table)

	table.Render()

	if !o.NoVersionCheck && !o.Verbose {
		return o.versionCheck()
	}

	return nil
}

func (o *VersionOptions) getVersionFromZitiApp(zitiApp string, versionArg string) (string, error) {
	if o.Verbose {
		return o.getCommandOutput("", zitiApp, versionArg, "--verbose")
	}
	return o.getCommandOutput("", zitiApp, versionArg)
}

func (o *VersionOptions) getVersionFromZitiAppMultiArg(zitiApp string) (string, error) {
	var varsionArg string
	if zitiApp == c.ZITI {
		varsionArg = "--version"
	} else {
		varsionArg = "version"
	}
	return o.getVersionFromZitiApp(zitiApp, varsionArg)
}

func (o *VersionOptions) versionPrintZitiApp(zitiApp string, table *table.Table) {
	output, err := o.getVersionFromZitiAppMultiArg(zitiApp)
	if err == nil {
		table.AddRow(zitiApp, util.ColorInfo(output))
	} else {
		if !isBinaryInstalled(zitiApp) {
			table.AddRow(zitiApp, util.ColorWarning("not installed"))
		}
	}
}

func (o *VersionOptions) versionCheck() error {
	o.versionCheckZitiApp(c.ZITI)
	o.versionCheckZitiApp(c.ZITI_CONTROLLER)
	o.versionCheckZitiApp(c.ZITI_FABRIC)
	o.versionCheckZitiApp(c.ZITI_FABRIC_TEST)
	o.versionCheckZitiApp(c.ZITI_FABRIC_GW)
	o.versionCheckZitiApp(c.ZITI_PROX_C)
	o.versionCheckZitiApp(c.ZITI_ROUTER)
	o.versionCheckZitiApp(c.ZITI_TUNNEL)
	o.versionCheckZitiApp(c.ZITI_EDGE_TUNNEL)
	return nil
}

func (o *VersionOptions) versionCheckZitiApp(zitiApp string) error {
	var currentVersion semver.Version
	var newVersion semver.Version
	var err error

	if !isBinaryInstalled(zitiApp) {
		return nil
	}

	switch zitiApp {
	case c.ZITI:
		newVersion, err = o.getLatestZitiAppVersion(version.GetBranch(), c.ZITI)
	case c.ZITI_CONTROLLER:
		newVersion, err = o.getLatestZitiAppVersion(version.GetBranch(), c.ZITI_CONTROLLER)
	case c.ZITI_FABRIC:
		newVersion, err = o.getLatestZitiAppVersion(version.GetBranch(), c.ZITI_FABRIC)
	case c.ZITI_FABRIC_TEST:
		newVersion, err = o.getLatestZitiAppVersion(version.GetBranch(), c.ZITI_FABRIC_TEST)
	case c.ZITI_FABRIC_GW:
		newVersion, err = o.getLatestZitiAppVersion(version.GetBranch(), c.ZITI_FABRIC_GW)
	case c.ZITI_PROX_C:
		newVersion, err = o.getLatestGitHubReleaseVersion(version.GetBranch(), c.ZITI_SDK_C_GITHUB)
	case c.ZITI_ROUTER:
		newVersion, err = o.getLatestZitiAppVersion(version.GetBranch(), c.ZITI_ROUTER)
	case c.ZITI_TUNNEL:
		newVersion, err = o.getLatestZitiAppVersion(version.GetBranch(), c.ZITI_TUNNEL)
	case c.ZITI_EDGE_TUNNEL:
		newVersion, err = o.getLatestGitHubReleaseVersion(version.GetBranch(), c.ZITI_EDGE_TUNNEL_GITHUB)
	default:
		return nil
	}
	if err != nil {
		return err
	}

	app := util.ColorInfo(zitiApp)
	output, err := o.getVersionFromZitiAppMultiArg(zitiApp)
	if err != nil {
		log.Warnf("\nAn err occurred: %s", err)
	} else {
		currentVersion, err = semver.ParseTolerant(output)
		if err != nil {
			log.Warnf("Failed to get %s version: %s\n", zitiApp, err)
			return err
		}
		if newVersion.GT(currentVersion) {
			log.Warnf("\nA new %s version is available: %s\n", app, util.ColorInfo(newVersion.String()))

			if o.BatchMode {
				log.Warnf("To upgrade to this new version use: %s\n", util.ColorInfo("ziti upgrade "+zitiApp))
			} else {
				message := fmt.Sprintf("Would you like to upgrade to the new %s version?", app)
				if util.Confirm(message, true, "Please indicate if you would like to upgrade the binary version.") {
					switch zitiApp {
					case c.ZITI:
						err = o.upgradeZiti()
					case c.ZITI_CONTROLLER:
						err = o.upgradeZitiController()
					case c.ZITI_FABRIC:
						err = o.upgradeZitiFabric()
					case c.ZITI_FABRIC_TEST:
						err = o.upgradeZitiFabricTest()
					case c.ZITI_FABRIC_GW:
						err = o.upgradeZitiMgmtGw()
					case c.ZITI_PROX_C:
						err = o.upgradeZitiProxC()
					case c.ZITI_ROUTER:
						err = o.upgradeZitiRouter()
					case c.ZITI_TUNNEL:
						err = o.upgradeZitiTunnel()
					case c.ZITI_EDGE_TUNNEL:
						err = o.upgradeZitiEdgeTunnel()
					default:
					}
					if err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}

func (o *VersionOptions) upgradeZiti() error {
	options := &UpgradeZitiOptions{
		CreateOptions: CreateOptions{
			CommonOptions: o.CommonOptions,
		},
	}
	return options.Run()
}

func (o *VersionOptions) upgradeZitiController() error {
	options := &UpgradeZitiControllerOptions{
		CreateOptions: CreateOptions{
			CommonOptions: o.CommonOptions,
		},
	}
	return options.Run()
}

func (o *VersionOptions) upgradeZitiFabric() error {
	options := &UpgradeZitiFabricOptions{
		CreateOptions: CreateOptions{
			CommonOptions: o.CommonOptions,
		},
	}
	return options.Run()
}

func (o *VersionOptions) upgradeZitiFabricTest() error {
	options := &UpgradeZitiFabricTestOptions{
		CreateOptions: CreateOptions{
			CommonOptions: o.CommonOptions,
		},
	}
	return options.Run()
}

func (o *VersionOptions) upgradeZitiMgmtGw() error {
	options := &UpgradeZitiMgmtGwOptions{
		CreateOptions: CreateOptions{
			CommonOptions: o.CommonOptions,
		},
	}
	return options.Run()
}

func (o *VersionOptions) upgradeZitiProxC() error {
	options := &UpgradeZitiProxCOptions{
		CreateOptions: CreateOptions{
			CommonOptions: o.CommonOptions,
		},
	}
	return options.Run()
}

func (o *VersionOptions) upgradeZitiRouter() error {
	options := &UpgradeZitiRouterOptions{
		CreateOptions: CreateOptions{
			CommonOptions: o.CommonOptions,
		},
	}
	return options.Run()
}

func (o *VersionOptions) upgradeZitiTunnel() error {
	options := &UpgradeZitiTunnelOptions{
		CreateOptions: CreateOptions{
			CommonOptions: o.CommonOptions,
		},
	}
	return options.Run()
}

func (o *VersionOptions) upgradeZitiEdgeTunnel() error {
	options := &UpgradeZitiEdgeTunnelOptions{
		CreateOptions: CreateOptions{
			CommonOptions: o.CommonOptions,
		},
	}
	return options.Run()
}
