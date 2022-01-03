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
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/common"
	cmdutil "github.com/openziti/ziti/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/spf13/cobra"
	"io"
)

const (
	optionRouterName = "routerName"
)

// CreateConfigRouterOptions the options for the create spring command
type CreateConfigRouterOptions struct {
	CreateConfigOptions

	RouterName string
}

// NewCmdCreateConfigRouter creates a command object for the "router" command
func NewCmdCreateConfigRouter(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &CreateConfigRouterOptions{
		CreateConfigOptions: CreateConfigOptions{
			CommonOptions: common.CommonOptions{
				Factory: f,
				Out:     out,
				Err:     errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:     "router",
		Short:   "Creates a config file for specified Router type",
		Aliases: []string{"rtr"},
		Run: func(cmd *cobra.Command, args []string) {
			cmdhelper.CheckErr(cmd.Help())
		},
	}

	cmd.AddCommand(NewCmdCreateConfigRouterEdge(common.NewOptionsProvider(out, errOut)))

	options.addFlags(cmd)
	return cmd
}

func (options *CreateConfigRouterOptions) addFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&options.RouterName, optionRouterName, "", "name of the router")
	err := cmd.MarkPersistentFlagRequired(optionRouterName)
	if err != nil {
		return
	}
}
