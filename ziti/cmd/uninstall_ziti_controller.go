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
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/templates"
	c "github.com/openziti/ziti/ziti/constants"
	"io"

	"github.com/spf13/cobra"
)

var (
	uninstallZitiControllerLong = templates.LongDesc(`
		UnInstalls the Ziti Controller app if it has previously been installed
`)

	uninstallZitiControllerExample = templates.Examples(`
		# UnInstall the Ziti Controller app 
		ziti uninstall ziti-controller
	`)
)

// UnInstallZitiControllerOptions the options for the upgrade ziti-controller command
type UnInstallZitiControllerOptions struct {
	UnInstallOptions

	Version string
}

// NewCmdUnInstallZitiController defines the command
func NewCmdUnInstallZitiController(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &UnInstallZitiControllerOptions{
		UnInstallOptions: UnInstallOptions{
			CommonOptions: CommonOptions{
				Out: out,
				Err: errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:     "ziti-controller",
		Short:   "UnInstalls the Ziti Controller app - if it has previously been installed",
		Aliases: []string{"controller"},
		Long:    uninstallZitiControllerLong,
		Example: uninstallZitiControllerExample,
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
func (o *UnInstallZitiControllerOptions) Run() error {
	o.deleteInstalledBinary(c.ZITI_CONTROLLER)
	return nil
}
