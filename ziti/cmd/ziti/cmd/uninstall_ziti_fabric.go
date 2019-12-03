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
	"github.com/spf13/cobra"
)

var (
	uninstallZitiFabricLong = templates.LongDesc(`
		UnInstalls the Ziti Fabric app if it has previously been installed
`)

	uninstallZitiFabricExample = templates.Examples(`
		# UnInstall the Ziti Fabric app 
		ziti uninstall ziti-fabric
	`)
)

// UnInstallZitiFabricOptions the options for the upgrade ziti-fabric command
type UnInstallZitiFabricOptions struct {
	UnInstallOptions

	Version string
}

// NewCmdUnInstallZitiFabric defines the command
func NewCmdUnInstallZitiFabric(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &UnInstallZitiFabricOptions{
		UnInstallOptions: UnInstallOptions{
			CommonOptions: CommonOptions{
				Factory: f,
				Out:     out,
				Err:     errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:     "ziti-fabric",
		Short:   "UnInstalls the Ziti Fabric app - if it has previously been installed",
		Aliases: []string{"fabric"},
		Long:    uninstallZitiFabricLong,
		Example: uninstallZitiFabricExample,
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
func (o *UnInstallZitiFabricOptions) Run() error {
	o.deleteInstalledBinary(c.ZITI_FABRIC)
	return nil
}
