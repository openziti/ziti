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
	"github.com/openziti/ziti/ziti/cmd/ziti/internal/log"
	"github.com/openziti/ziti/common/version"
	"github.com/blang/semver"
	"github.com/spf13/cobra"
)

var (
	installZitiProxyLong = templates.LongDesc(`
		Installs the Ziti Proxy app if it has not been installed already
`)

	installZitiProxyExample = templates.Examples(`
		# Install the Ziti Proxy app 
		ziti install ziti-proxy
	`)
)

// InstallZitiProxyOptions the options for the upgrade ziti-proxy command
type InstallZitiProxyOptions struct {
	InstallOptions

	Version string
}

// NewCmdInstallZitiProxy defines the command
func NewCmdInstallZitiProxy(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &InstallZitiProxyOptions{
		InstallOptions: InstallOptions{
			CommonOptions: CommonOptions{
				Factory: f,
				Out:     out,
				Err:     errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:     "ziti-proxy",
		Short:   "Installs the Ziti Proxy app - if it has not been installed already",
		Aliases: []string{"proxy"},
		Long:    installZitiProxyLong,
		Example: installZitiProxyExample,
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
func (o *InstallZitiProxyOptions) Run() error {
	newVersion, err := o.getLatestZitiAppVersion(version.GetBranch(), c.ZITI_PROXY)
	if err != nil {
		return err
	}

	if o.Version != "" {
		newVersion, err = semver.Make(o.Version)
	}

	log.Infoln("Attempting to install '" + c.ZITI_PROXY + "' version: " + newVersion.String())

	return o.installZitiApp(version.GetBranch(), c.ZITI_PROXY, false, newVersion.String())
}
