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
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/openziti/fabric/router"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/agentcli"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/spf13/cobra"
)

type AgentAppId byte

const (
	AgentAppController = AgentAppId(controller.AgentAppId)
	AgentAppRouter     = AgentAppId(router.AgentAppId)
)

func NewAgentCmd(p common.OptionsProvider) *cobra.Command {
	agentCmd := agentcli.NewAgentCmd(p)

	ctrlCmd := &cobra.Command{
		Use:     "controller",
		Aliases: []string{"c"},
		Short:   "Interact with a ziti-controller process using the IPC agent",
		Run: func(cmd *cobra.Command, args []string) {
			cmdhelper.CheckErr(cmd.Help())
		},
	}

	agentCmd.AddCommand(ctrlCmd)
	ctrlCmd.AddCommand(NewSimpleChAgentCustomCmd("snapshot-db", AgentAppController, int32(mgmt_pb.ContentType_SnapshotDbRequestType), p))

	routerCmd := &cobra.Command{
		Use:     "router",
		Aliases: []string{"r"},
		Short:   "Interact with a ziti-router process using the IPC agent",
		Run: func(cmd *cobra.Command, args []string) {
			cmdhelper.CheckErr(cmd.Help())
		},
	}

	agentCmd.AddCommand(routerCmd)

	routerCmd.AddCommand(NewRouteCmd(p))
	routerCmd.AddCommand(NewSimpleAgentCustomCmd("dump-routes", AgentAppRouter, router.DumpForwarderTables, p))
	routerCmd.AddCommand(NewSimpleAgentCustomCmd("disconnect", AgentAppRouter, router.CloseControlChannel, p))
	routerCmd.AddCommand(NewSimpleAgentCustomCmd("reconnect", AgentAppRouter, router.OpenControlChannel, p))
	routerCmd.AddCommand(NewSimpleAgentCustomCmd("dump-api-sessions", AgentAppRouter, debugops.DumpApiSessions, p))
	routerCmd.AddCommand(NewSimpleAgentCustomCmd("dump-links", AgentAppRouter, router.DumpLinks, p))
	routerCmd.AddCommand(NewForgetLinkAgentCmd(p))

	return agentCmd
}
