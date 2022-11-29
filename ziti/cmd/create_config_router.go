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
	_ "embed"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const (
	optionRouterName = "routerName"
)

// CreateConfigRouterOptions the options for the router command
type CreateConfigRouterOptions struct {
	CreateConfigOptions

	RouterName   string
	WssEnabled   bool
	IsPrivate    bool
	TunnelerMode string
	LanInterface string
}

var routerOptions = CreateConfigRouterOptions{}

// NewCmdCreateConfigRouter creates a command object for the "router" command
func NewCmdCreateConfigRouter() *cobra.Command {

	cmd := &cobra.Command{
		Use:     "router",
		Short:   "Creates a config file for specified Router name",
		Aliases: []string{"rtr"},
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Setup logging
			var logOut *os.File
			if routerOptions.Verbose {
				logrus.SetLevel(logrus.DebugLevel)
				// Only print log to stdout if not printing config to stdout
				if strings.ToLower(routerOptions.Output) != "stdout" {
					logOut = os.Stdout
				} else {
					logOut = os.Stderr
				}
				logrus.SetOutput(logOut)
			}

			data.populateEnvVars()
			data.populateDefaults()

			// Update router data with options passed in
			data.Router.Name = validateRouterName(routerOptions.RouterName)
			SetZitiRouterIdentity(&data.Router, data.Router.Name)
		},
		Run: func(cmd *cobra.Command, args []string) {
			cmdhelper.CheckErr(cmd.Help())
		},
	}

	cmd.AddCommand(NewCmdCreateConfigRouterEdge())
	cmd.AddCommand(NewCmdCreateConfigRouterFabric())

	routerOptions.addCreateFlags(cmd)
	routerOptions.addFlags(cmd)
	return cmd
}

func (options *CreateConfigRouterOptions) addFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVarP(&options.RouterName, optionRouterName, "n", "", "name of the router")
	err := cmd.MarkPersistentFlagRequired(optionRouterName)
	if err != nil {
		return
	}
}
