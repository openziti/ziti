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
	"fmt"
	"github.com/blang/semver"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/openziti/ziti/ziti/cmd/table"
	"github.com/openziti/ziti/ziti/internal/log"
	"io"
	"os"

	"github.com/openziti/ziti/common/version"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	c "github.com/openziti/ziti/ziti/constants"
	"github.com/openziti/ziti/ziti/util"

	"github.com/spf13/cobra"
)

type VersionOptions struct {
	InstallOptions

	Container      string
	NoVersionCheck bool
}

func NewCmdVersion(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &VersionOptions{
		InstallOptions: InstallOptions{
			CommonOptions: common.CommonOptions{
				Out: out,
				Err: errOut,
			},
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
		Hidden: true,
	}
	options.AddCommonFlags(cmd)

	cmd.Flags().BoolVarP(&options.NoVersionCheck, "no-update", "n", false,
		"disable update check")

	return cmd
}

// Run ...
func (o *VersionOptions) Run() error {

	util.ConfigDir()

	info := util.ColorInfo

	t := table.CreateTable(os.Stdout)
	t.AddRow("NAME", "VERSION")

	t.AddRow(c.ZITI, info(version.GetBuildMetadata(o.Verbose)))

	o.versionPrintZitiApp(c.ZITI_CONTROLLER, &t)
	o.versionPrintZitiApp(c.ZITI_PROX_C, &t)
	o.versionPrintZitiApp(c.ZITI_ROUTER, &t)
	o.versionPrintZitiApp(c.ZITI_TUNNEL, &t)
	o.versionPrintZitiApp(c.ZITI_EDGE_TUNNEL, &t)

	t.Render()

	if !o.NoVersionCheck && !o.Verbose {
		return o.versionCheck()
	}

	return nil
}

func (o *VersionOptions) getVersionFromZitiApp(zitiApp string, versionArg string) (string, error) {
	if o.Verbose {
		return o.GetCommandOutput("", zitiApp, versionArg, "--verbose")
	}
	return o.GetCommandOutput("", zitiApp, versionArg)
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
	case c.ZITI_PROX_C:
		newVersion, err = o.getLatestGitHubReleaseVersion(c.ZITI_SDK_C_GITHUB)
	case c.ZITI_ROUTER:
		newVersion, err = o.getLatestZitiAppVersion(version.GetBranch(), c.ZITI_ROUTER)
	case c.ZITI_TUNNEL:
		newVersion, err = o.getLatestZitiAppVersion(version.GetBranch(), c.ZITI_TUNNEL)
	case c.ZITI_EDGE_TUNNEL:
		newVersion, err = o.getLatestGitHubReleaseVersion(c.ZITI_EDGE_TUNNEL_GITHUB)
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
		InstallOptions: InstallOptions{
			CommonOptions: o.CommonOptions,
		},
	}
	return options.Run()
}

func (o *VersionOptions) upgradeZitiController() error {
	options := &UpgradeZitiControllerOptions{
		InstallOptions: InstallOptions{
			CommonOptions: o.CommonOptions,
		},
	}
	return options.Run()
}

func (o *VersionOptions) upgradeZitiProxC() error {
	options := &UpgradeZitiProxCOptions{
		InstallOptions: InstallOptions{
			CommonOptions: o.CommonOptions,
		},
	}
	return options.Run()
}

func (o *VersionOptions) upgradeZitiRouter() error {
	options := &UpgradeZitiRouterOptions{
		InstallOptions: InstallOptions{
			CommonOptions: o.CommonOptions,
		},
	}
	return options.Run()
}

func (o *VersionOptions) upgradeZitiTunnel() error {
	options := &UpgradeZitiTunnelOptions{
		InstallOptions: InstallOptions{
			CommonOptions: o.CommonOptions,
		},
	}
	return options.Run()
}

func (o *VersionOptions) upgradeZitiEdgeTunnel() error {
	options := &UpgradeZitiEdgeTunnelOptions{
		InstallOptions: InstallOptions{
			CommonOptions: o.CommonOptions,
		},
	}
	return options.Run()
}
