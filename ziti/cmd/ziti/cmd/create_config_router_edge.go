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
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/templates"
	"github.com/spf13/cobra"
)

var (
	createConfigRouterEdgeLong = templates.LongDesc(`
		Creates the edge router config
`)

	createConfigRouterEdgeExample = templates.Examples(`
		# Create the edge router config for a router named my_router
		ziti create config router edge --name my_router
	`)
)

// CreateConfigRouterEdgeOptions the options for the create spring command
type CreateConfigRouterEdgeOptions struct {
	CreateConfigRouterOptions
}

// NewCmdCreateConfigRouterEdge creates a command object for the "edge" command
func NewCmdCreateConfigRouterEdge(p common.OptionsProvider) *cobra.Command {
	options := &CreateConfigRouterEdgeOptions{
		CreateConfigRouterOptions: CreateConfigRouterOptions{
			CreateConfigOptions: CreateConfigOptions{
				CommonOptions: p(),
			},
		},
	}

	cmd := &cobra.Command{
		Use:     "edge",
		Short:   "Create an edge router config",
		Aliases: []string{"edge"},
		Long:    createConfigRouterEdgeLong,
		Example: createConfigRouterEdgeExample,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}

	options.addFlags(cmd)

	return cmd
}

func (options *CreateConfigRouterEdgeOptions) addFlags(cmd *cobra.Command) {
	// TODO: implement
}

// Run implements the command
func (options *CreateConfigRouterEdgeOptions) Run() error {

	return fmt.Errorf("UNIMPLEMENTED: '%s'", "create config router edge")

	// return nil
}
