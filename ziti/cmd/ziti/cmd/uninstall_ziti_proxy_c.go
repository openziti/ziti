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

package cmd

import (
	"io"

	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/templates"
	c "github.com/openziti/ziti/ziti/cmd/ziti/constants"
	"github.com/spf13/cobra"
)

var (
	uninstallZitiProxCLong = templates.LongDesc(`
		UnInstalls the Ziti ProxC app if it has previously been installed
`)

	uninstallZitiProxCExample = templates.Examples(`
		# UnInstall the Ziti ProxC app 
		ziti uninstall ziti-prox-c
	`)
)

// UnInstallZitiProxCOptions the options for the upgrade ziti-prox-c command
type UnInstallZitiProxCOptions struct {
	UnInstallOptions

	Version string
}

// NewCmdUnInstallZitiProxC defines the command
func NewCmdUnInstallZitiProxC(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &UnInstallZitiProxCOptions{
		UnInstallOptions: UnInstallOptions{
			CommonOptions: CommonOptions{
				Out: out,
				Err: errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:     "ziti-prox-c",
		Short:   "UnInstalls the Ziti ProxC app - if it has previously been installed",
		Aliases: []string{"proxc"},
		Long:    uninstallZitiProxCLong,
		Example: uninstallZitiProxCExample,
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
func (o *UnInstallZitiProxCOptions) Run() error {
	o.deleteInstalledBinary(c.ZITI_PROX_C)
	return nil
}
