package agentcli

import (
	"github.com/openziti/channel"
	"github.com/openziti/agent"
	"github.com/openziti/identity"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/spf13/cobra"
	"net"
	"time"
)

func NewAgentCmd(p common.OptionsProvider) *cobra.Command {
	agentCmd := &cobra.Command{
		Use:   "agent",
		Short: "Interact with ziti processes using the the IPC agent",
		Run: func(cmd *cobra.Command, args []string) {
			cmdhelper.CheckErr(cmd.Help())
		},
	}

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
