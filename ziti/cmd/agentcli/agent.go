package agentcli

import (
	"github.com/openziti/agent"
	"github.com/openziti/channel/v2"
	"github.com/openziti/edge/router/debugops"
	"github.com/openziti/fabric/controller"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/openziti/fabric/router"
	"github.com/openziti/identity"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/spf13/cobra"
	"net"
	"time"
)

type AgentAppId byte

const (
	AgentAppController = AgentAppId(controller.AgentAppId)
	AgentAppRouter     = AgentAppId(router.AgentAppId)
)

func NewAgentCmd(p common.OptionsProvider) *cobra.Command {
	agentCmd := &cobra.Command{
		Use:   "agent",
		Short: "Interact with ziti processes using the the IPC agent",
		Run: func(cmd *cobra.Command, args []string) {
			cmdhelper.CheckErr(cmd.Help())
		},
	}

	agentCmd.AddCommand(NewPsCmd(p))
	agentCmd.AddCommand(NewGoVersionCmd(p))
	agentCmd.AddCommand(NewGcCmd(p))
	agentCmd.AddCommand(NewSetGcCmd(p))
	agentCmd.AddCommand(NewStackCmd(p))
	agentCmd.AddCommand(NewMemstatsCmd(p))
	agentCmd.AddCommand(NewStatsCmd(p))
	agentCmd.AddCommand(NewPprofHeapCmd(p))
	agentCmd.AddCommand(NewPprofCpuCmd(p))
	agentCmd.AddCommand(NewTraceCmd(p))
	agentCmd.AddCommand(NewSetLogLevelCmd(p))
	agentCmd.AddCommand(NewSetChannelLogLevelCmd(p))
	agentCmd.AddCommand(NewClearChannelLogLevelCmd(p))

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
	ctrlCmd.AddCommand(NewAgentCtrlRaftJoin(p))
	ctrlCmd.AddCommand(NewAgentCtrlRaftList(p))
	ctrlCmd.AddCommand(NewAgentCtrlInit(p))

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

// AgentOptions contains the command line options
type AgentOptions struct {
	common.CommonOptions
}

func NewAgentChannel(conn net.Conn) (channel.Channel, error) {
	options := channel.DefaultOptions()
	options.ConnectTimeout = time.Second
	dialer := channel.NewExistingConnDialer(&identity.TokenId{Token: "agent"}, conn, nil)
	return channel.NewChannel("agent", dialer, nil, options)
}

func MakeAgentChannelRequest(addr string, signal byte, params []byte, f func(ch channel.Channel) error) error {
	return agent.MakeRequestF(addr, signal, params, func(conn net.Conn) error {
		ch, err := NewAgentChannel(conn)
		if err != nil {
			return err
		}
		return f(ch)
	})
}
