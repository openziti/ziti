/*
	Copyright 2019 NetFoundry, Inc.

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

var (
	createConfigMgmtLong = templates.LongDesc(`
		Creates the mgmt config
`)

	createConfigMgmtExample = templates.Examples(`
		# Create the mgmt config 
		ziti create config mgmt

		# Create the mgmt config with a particular ctrlListener
		ziti create config mgmt -ctrlListener quic:0.0.0.0:6262
	`)
)

// CreateConfigMgmtOptions the options for the create spring command
type CreateConfigMgmtOptions struct {
	CreateConfigOptions

	CtrlAddress string
}

// NewCmdCreateConfigMgmt creates a command object for the "create" command
func NewCmdCreateConfigMgmt(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &CreateConfigMgmtOptions{
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
		Use:     "mgmt",
		Short:   "Create a mgmt config",
		Aliases: []string{"m"},
		Long:    createConfigMgmtLong,
		Example: createConfigMgmtExample,
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
func (o *CreateConfigMgmtOptions) Run() error {
	if o.CtrlAddress == "" {
		return util.MissingOption(optionCtrlAddress)
	}

	return fmt.Errorf("UNIMPLEMENTED: '%s'", "create config mgmt")

	// return nil
}
