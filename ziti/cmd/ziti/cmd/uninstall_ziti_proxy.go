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
	"github.com/spf13/cobra"
)

var (
	uninstallZitiProxyLong = templates.LongDesc(`
		UnInstalls the Ziti Proxy app if it has previously been installed
`)

	uninstallZitiProxyExample = templates.Examples(`
		# UnInstall the Ziti Proxy app 
		ziti uninstall ziti-proxy
	`)
)

// UnInstallZitiProxyOptions the options for the upgrade ziti-proxy command
type UnInstallZitiProxyOptions struct {
	UnInstallOptions

	Version string
}

// NewCmdUnInstallZitiProxy defines the command
func NewCmdUnInstallZitiProxy(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &UnInstallZitiProxyOptions{
		UnInstallOptions: UnInstallOptions{
			CommonOptions: CommonOptions{
				Factory: f,
				Out:     out,
				Err:     errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:     "ziti-proxy",
		Short:   "UnInstalls the Ziti Proxy app - if it has previously been installed",
		Aliases: []string{"proxy"},
		Long:    uninstallZitiProxyLong,
		Example: uninstallZitiProxyExample,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}
	return cmd
}

// Run implements the command
func (o *UnInstallZitiProxyOptions) Run() error {
	o.deleteInstalledBinary(c.ZITI_PROXY)
	return nil
}
