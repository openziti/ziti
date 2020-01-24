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
	c "github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/constants"
	"github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/internal/log"
	"github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/util"
)

const (
	optionListenAddress  = "listenAddress"
	defaultListenAddress = "localhost:10080"

	optionMgmt  = "mgmt"
	defaultMgmt = "tls:0.0.0.0:10000"

	optionMgmtGwCertPath = "mgmtGwCertPath"

	optionMgmtGwKeyPath = "mgmtGwKeyPath"

	optionMgmtCertPath = "mgmtCertPath"

	optionMgmtKeyPath = "mgmtKeyPath"

	optionMgmtCaCertPath = "mgmtCaCertPath"
)

var (
	createConfigGwLong = templates.LongDesc(`
		Creates the gw config
	`)

	createConfigGwExample = templates.Examples(`
		# Create the gw config 
		ziti create config gw

		# Create the gw config with a particular ctrlListener
		ziti create config gw -listenAddress localhost:10080
	`)
)

// CreateConfigGwOptions the options for the create spring command
type CreateConfigGwOptions struct {
	CreateConfigOptions

	ListenAddress  string
	Mgmt           string
	MgmtGwCertPath string
	MgmtGwKeyPath  string
	MgmtCertPath   string
	MgmtKeyPath    string
	MgmtCaCertPath string
}

// NewCmdCreateConfigGw creates a command object for the "create" command
func NewCmdCreateConfigGw(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &CreateConfigGwOptions{
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
		Use:     "gw",
		Short:   "Create a gw config",
		Aliases: []string{"g"},
		Long:    createConfigGwLong,
		Example: createConfigGwExample,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}

	options.addCommonFlags(cmd)
	options.addFlags(cmd, "", defaultCtrlListener)

	cmd.Flags().StringVarP(&options.ListenAddress, optionListenAddress, "l", "", "Where the GW should listen for incoming REST requests")
	return cmd
}

// Run implements the command
func (o *CreateConfigGwOptions) Run() error {
	if o.ListenAddress == "" {
		log.Infof("%s not specified; using default (%s)\n", optionListenAddress, defaultListenAddress)
		o.ListenAddress = defaultListenAddress
		// return util.MissingOption(optionListenAddress)
	}

	gwConfigDir, err := util.ZitiAppConfigDir(c.ZITI_FABRIC_GW)

	if err != nil {
		return fmt.Errorf("err")
	}

	return fmt.Errorf("UNIMPLEMENTED: '%s'", gwConfigDir)
}
