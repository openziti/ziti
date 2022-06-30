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
	uninstallZitiRouterLong = templates.LongDesc(`
		UnInstalls the Ziti Router app if it has previously been installed
`)

	uninstallZitiRouterExample = templates.Examples(`
		# UnInstall the Ziti Router app 
		ziti uninstall ziti-router
	`)
)

// UnInstallZitiRouterOptions the options for the upgrade ziti-router command
type UnInstallZitiRouterOptions struct {
	UnInstallOptions

	Version string
}

// NewCmdUnInstallZitiRouter defines the command
func NewCmdUnInstallZitiRouter(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &UnInstallZitiRouterOptions{
		UnInstallOptions: UnInstallOptions{
			CommonOptions: CommonOptions{
				Out: out,
				Err: errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:     "ziti-router",
		Short:   "UnInstalls the Ziti Router app - if it has previously been installed",
		Aliases: []string{"router"},
		Long:    uninstallZitiRouterLong,
		Example: uninstallZitiRouterExample,
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
func (o *UnInstallZitiRouterOptions) Run() error {
	o.deleteInstalledBinary(c.ZITI_ROUTER)
	return nil
}
