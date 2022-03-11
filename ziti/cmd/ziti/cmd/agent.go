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
	"github.com/openziti/edge/router/debugops"
	"github.com/openziti/fabric/controller"
	"github.com/openziti/fabric/router"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/spf13/cobra"
	"io"
)

type AgentAppId byte

const (
	AgentAppController = AgentAppId(controller.AgentAppId)
	AgentAppRouter     = AgentAppId(router.AgentAppId)
)

func NewAgentCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	agentCmd := &cobra.Command{
		Use:   "agent",
		Short: "Interact with ziti processes using the the IPC agent",
		Run: func(cmd *cobra.Command, args []string) {
			cmdhelper.CheckErr(cmd.Help())
		},
	}

	ctrlCmd := &cobra.Command{
		Use:     "controller",
		Aliases: []string{"c"},
		Short:   "Interact with a ziti-controller process using the IPC agent",
		Run: func(cmd *cobra.Command, args []string) {
			cmdhelper.CheckErr(cmd.Help())
		},
	}

	agentCmd.AddCommand(ctrlCmd)
	ctrlCmd.AddCommand(NewSimpleAgentCustomCmd("snapshot-db", AgentAppController, controller.AgentOpSnapshotDbSnaps, out, errOut))

	routerCmd := &cobra.Command{
		Use:     "router",
		Aliases: []string{"r"},
		Short:   "Interact with a ziti-router process using the IPC agent",
		Run: func(cmd *cobra.Command, args []string) {
			cmdhelper.CheckErr(cmd.Help())
		},
	}

	agentCmd.AddCommand(routerCmd)

	routerCmd.AddCommand(NewCmdPsRoute(out, errOut))
	routerCmd.AddCommand(NewSimpleAgentCustomCmd("dump-routes", AgentAppRouter, router.DumpForwarderTables, out, errOut))
	routerCmd.AddCommand(NewSimpleAgentCustomCmd("disconnect", AgentAppRouter, router.CloseControlChannel, out, errOut))
	routerCmd.AddCommand(NewSimpleAgentCustomCmd("reconnect", AgentAppRouter, router.OpenControlChannel, out, errOut))
	routerCmd.AddCommand(NewSimpleAgentCustomCmd("dump-api-sessions", AgentAppRouter, debugops.DumpApiSessions, out, errOut))
	routerCmd.AddCommand(NewSimpleAgentCustomCmd("dump-links", AgentAppRouter, router.DumpLinks, out, errOut))

	agentCmd.AddCommand(NewCmdPsGoversion(out, errOut))
	agentCmd.AddCommand(NewCmdPsGc(out, errOut))
	agentCmd.AddCommand(NewCmdPsSetgc(out, errOut))
	agentCmd.AddCommand(NewCmdPsStack(out, errOut))
	agentCmd.AddCommand(NewCmdPsMemstats(out, errOut))
	agentCmd.AddCommand(NewCmdPsStats(out, errOut))
	agentCmd.AddCommand(NewCmdPsPprofHeap(out, errOut))
	agentCmd.AddCommand(NewCmdPsPprofCpu(out, errOut))
	agentCmd.AddCommand(NewCmdPsTrace(out, errOut))
	agentCmd.AddCommand(NewCmdPsSetLogLevel(out, errOut))
	agentCmd.AddCommand(NewCmdPsSetChannelLogLevel(out, errOut))
	agentCmd.AddCommand(NewCmdPsClearChannelLogLevel(out, errOut))

	return agentCmd
}
