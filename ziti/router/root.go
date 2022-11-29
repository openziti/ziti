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

package router

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/ziti/constants"
	"github.com/openziti/ziti/ziti/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func NewRouterCmd() *cobra.Command {
	var routerCmd = &cobra.Command{
		Use:   "router",
		Short: "Ziti Router",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if verbose {
				logrus.SetLevel(logrus.DebugLevel)
			}

			switch logFormatter {
			case "pfxlog":
				pfxlog.SetFormatter(pfxlog.NewFormatter(pfxlog.DefaultOptions().SetTrimPrefix("github.com/openziti/").StartingToday()))
			case "json":
				pfxlog.SetFormatter(&logrus.JSONFormatter{TimestampFormat: "2006-01-02T15:04:05.000Z"})
			case "text":
				pfxlog.SetFormatter(&logrus.TextFormatter{})
			default:
				// let logrus do its own thing
			}

			util.LogReleaseVersionCheck(constants.ZITI_ROUTER)
		},
	}

	routerCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")
	routerCmd.PersistentFlags().StringVar(&logFormatter, "log-formatter", "", "Specify log formatter [json|pfxlog|text]")
	routerCmd.PersistentFlags().BoolVar(&cliAgentEnabled, "cli-agent", true, "Enable/disable CLI Agent (enabled by default)")
	routerCmd.PersistentFlags().StringVar(&cliAgentAddr, "cli-agent-addr", "", "Specify where CLI Agent should list (ex: unix:/tmp/myfile.sock or tcp:127.0.0.1:10001)")
	routerCmd.PersistentFlags().BoolVar(&enableDebugOps, "debug-ops", false, "Enable/disable debug agent operations (disabled by default)")

	routerCmd.AddCommand(NewRunCmd())
	routerCmd.AddCommand(NewEnrollGwCmd())
	routerCmd.AddCommand(NewVersionCmd())

	return routerCmd
}

var verbose bool
var logFormatter string
var cliAgentEnabled bool
var enableDebugOps bool
var cliAgentAddr string

func Execute() {
	if err := NewRouterCmd().Execute(); err != nil {
		fmt.Printf("error: %s\n", err)
	}
}
