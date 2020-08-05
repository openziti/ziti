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
	uninstallZitiFabricTestLong = templates.LongDesc(`
		UnInstalls the Ziti Fabric Test app if it has previously been installed
`)

	uninstallZitiFabricTestExample = templates.Examples(`
		# UnInstall the Ziti Fabric Test app 
		ziti uninstall ziti-fabric-test
	`)
)

// UnInstallZitiFabricTestOptions the options for the upgrade ziti-channel command
type UnInstallZitiFabricTestOptions struct {
	UnInstallOptions

	Version string
}

// NewCmdUnInstallZitiFabricTest defines the command
func NewCmdUnInstallZitiFabricTest(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &UnInstallZitiFabricTestOptions{
		UnInstallOptions: UnInstallOptions{
			CommonOptions: CommonOptions{
				Factory: f,
				Out:     out,
				Err:     errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:     "ziti-fabric-test",
		Short:   "UnInstalls the Ziti Fabric Test app - if it has previously been installed",
		Aliases: []string{"ft"},
		Long:    uninstallZitiFabricTestLong,
		Example: uninstallZitiFabricTestExample,
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
func (o *UnInstallZitiFabricTestOptions) Run() error {
	o.deleteInstalledBinary(c.ZITI_FABRIC_TEST)
	return nil
}
