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
	"github.com/spf13/cobra"
	"io"

	cmdutil "github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/helpers"
	"github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/templates"
	"github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/util"
)

const (
	optionCtrlAddress  = "ctrlAddress"
	defaultCtrlAddress = "quic:0.0.0.0:6262"
)

var (
	createConfigRouterLong = templates.LongDesc(`
		Creates the router config
`)

	createConfigRouterExample = templates.Examples(`
		# Create the router config 
		ziti create config router

		# Create the router config with a particular ctrlListener
		ziti create config router -ctrlListener quic:0.0.0.0:6262
	`)
)

// CreateConfigRouterOptions the options for the create spring command
type CreateConfigRouterOptions struct {
	CreateConfigOptions

	CtrlAddress string
}

// NewCmdCreateConfigRouter creates a command object for the "create" command
func NewCmdCreateConfigRouter(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &CreateConfigRouterOptions{
		CreateConfigOptions: CreateConfigOptions{
			CreateOptions: CreateOptions{
				CommonOptions: CommonOptions{
					Factory: f,
					Out:     out,
					Err:     errOut,
				},
			},
		},
	}

	cmd := &cobra.Command{
		Use:     "router",
		Short:   "Create a router config",
		Aliases: []string{"rtr"},
		Long:    createConfigRouterLong,
		Example: createConfigRouterExample,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}

	options.addCommonFlags(cmd)
	options.addFlags(cmd, "", defaultCtrlListener)

	return cmd
}

// Run implements the command
func (o *CreateConfigRouterOptions) Run() error {
	if o.CtrlAddress == "" {
		return util.MissingOption(optionCtrlAddress)
	}

	return fmt.Errorf("UNIMPLEMENTED: '%s'", "create config router")

	// return nil
}
