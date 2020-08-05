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
	"github.com/spf13/cobra"
)

var (
	uninstallZitiEnrollerLong = templates.LongDesc(`
		UnInstalls the Ziti Enroller app if it has previously been installed
`)

	uninstallZitiEnrollerExample = templates.Examples(`
		# UnInstall the Ziti Enroller app 
		ziti uninstall ziti-enroller
	`)
)

// UnInstallZitiEnrollerOptions the options for the upgrade ziti-enroller command
type UnInstallZitiEnrollerOptions struct {
	UnInstallOptions

	Version string
}

// NewCmdUnInstallZitiEnroller defines the command
func NewCmdUnInstallZitiEnroller(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &UnInstallZitiEnrollerOptions{
		UnInstallOptions: UnInstallOptions{
			CommonOptions: CommonOptions{
				Factory: f,
				Out:     out,
				Err:     errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:     "ziti-enroller",
		Short:   "UnInstalls the Ziti Enroller app - if it has previously been installed",
		Aliases: []string{"enroller"},
		Long:    uninstallZitiEnrollerLong,
		Example: uninstallZitiEnrollerExample,
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
func (o *UnInstallZitiEnrollerOptions) Run() error {
	o.deleteInstalledBinary(c.ZITI_ENROLLER)
	return nil
}
