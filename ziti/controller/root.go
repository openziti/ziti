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

package controller

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	edgeSubCmd "github.com/openziti/edge/controller/subcmd"
	"github.com/openziti/ziti/common/version"
	"github.com/openziti/ziti/ziti/constants"
	"github.com/openziti/ziti/ziti/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func NewControllerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "controller",
		Short: "Ziti Controller",
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

			util.LogReleaseVersionCheck(constants.ZITI_CONTROLLER)
		},
	}

	cmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")
	cmd.PersistentFlags().BoolVarP(&cliAgentEnabled, "cliagent", "a", true, "Enable/disabled CLI Agent (enabled by default)")
	cmd.PersistentFlags().StringVar(&cliAgentAddr, "cli-agent-addr", "", "Specify where CLI Agent should list (ex: unix:/tmp/myfile.sock or tcp:127.0.0.1:10001)")
	cmd.PersistentFlags().StringVar(&logFormatter, "log-formatter", "", "Specify log formatter [json|pfxlog|text]")
	cmd.PersistentFlags().BoolVar(&syncRaftToDb, "sync-raft-to-db", false, "Sync the current database state to raft as a snapshot. Use when moving an existing controller to run in HA/Raft")

	cmd.AddCommand(NewRunCmd())
	cmd.AddCommand(NewDeleteSessionsCmd())
	cmd.AddCommand(NewVersionCmd())

	edgeSubCmd.AddCommands(cmd, version.GetCmdBuildInfo())

	return cmd
}

var verbose bool
var cliAgentEnabled bool
var cliAgentAddr string
var logFormatter string
var syncRaftToDb bool

func Execute() {
	if err := NewControllerCmd().Execute(); err != nil {
		fmt.Printf("error: %s\n", err)
	}
}
