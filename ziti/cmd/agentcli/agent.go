package agentcli

import (
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"github.com/openziti/agent"
	"github.com/openziti/channel/v2"
	"github.com/openziti/identity"
	"github.com/openziti/ziti/common/pb/mgmt_pb"
	"github.com/openziti/ziti/controller"
	"github.com/openziti/ziti/router"
	"github.com/openziti/ziti/router/debugops"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
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
	agentCmd.AddCommand(NewListCmd(p))
	agentCmd.AddCommand(NewSimpleAgentCmd("goversion", agent.Version, p, "Returns the golang version of the target application"))
	agentCmd.AddCommand(NewSimpleAgentCmd("gc", agent.GC, p, "Run garbage collection in the target application"))
	agentCmd.AddCommand(NewSetGcCmd(p))
	agentCmd.AddCommand(NewStackCmd(p))
	agentCmd.AddCommand(NewSimpleAgentCmd("memstats", agent.MemStats, p, "Returns memory use summary of the target application"))
	agentCmd.AddCommand(NewSimpleAgentCmd("stats", agent.Stats, p, "Emits some runtime information (# go-routines, threads, cpus, etc) from the target application"))
	agentCmd.AddCommand(NewPprofHeapCmd(p))
	agentCmd.AddCommand(NewPprofCpuCmd(p))
	agentCmd.AddCommand(NewTraceCmd(p))
	agentCmd.AddCommand(NewSetLogLevelCmd(p))
	agentCmd.AddCommand(NewSetChannelLogLevelCmd(p))
	agentCmd.AddCommand(NewClearChannelLogLevelCmd(p))

	ctrlCmd := &cobra.Command{
		Use:     "controller",
		Aliases: []string{"c"},
		Short:   "Interact with a ziti controller process using the IPC agent",
		Run: func(cmd *cobra.Command, args []string) {
			cmdhelper.CheckErr(cmd.Help())
		},
	}

	agentCmd.AddCommand(ctrlCmd)
	ctrlCmd.AddCommand(NewSimpleChAgentCustomCmd("snapshot-db", AgentAppController, int32(mgmt_pb.ContentType_SnapshotDbRequestType), p))
	ctrlCmd.AddCommand(NewAgentCtrlInit(p))
	ctrlCmd.AddCommand(NewAgentCtrlInitFromDb(p))

	clusterCmd := &cobra.Command{
		Use:   "cluster",
		Short: "Manage an HA controller cluster using the IPC agent",
		Run: func(cmd *cobra.Command, args []string) {
			cmdhelper.CheckErr(cmd.Help())
		},
	}
	agentCmd.AddCommand(clusterCmd)
	clusterCmd.AddCommand(NewAgentClusterAdd(p))
	clusterCmd.AddCommand(NewAgentClusterRemove(p))
	clusterCmd.AddCommand(NewAgentClusterList(p))
	clusterCmd.AddCommand(NewAgentTransferLeadership(p))

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
	routerCmd.AddCommand(NewUnrouteCmd(p))
	routerCmd.AddCommand(NewSimpleAgentCustomCmd("dump-api-sessions", AgentAppRouter, debugops.DumpApiSessions, p))
	routerCmd.AddCommand(NewSimpleChAgentCustomCmd("dump-routes", AgentAppRouter, int32(mgmt_pb.ContentType_RouterDebugDumpForwarderTablesRequestType), p))
	routerCmd.AddCommand(NewSimpleChAgentCustomCmd("dump-links", AgentAppRouter, int32(mgmt_pb.ContentType_RouterDebugDumpLinksRequestType), p))
	routerCmd.AddCommand(NewForgetLinkAgentCmd(p))
	routerCmd.AddCommand(NewToggleCtrlChannelAgentCmd(p, "disconnect", false))
	routerCmd.AddCommand(NewToggleCtrlChannelAgentCmd(p, "reconnect", true))

	quiesceCmd := NewSimpleChAgentCustomCmd("quiesce", AgentAppRouter, int32(mgmt_pb.ContentType_RouterQuiesceRequestType), p)
	quiesceCmd.Hidden = true
	routerCmd.AddCommand(quiesceCmd)

	dequiesceCmd := NewSimpleChAgentCustomCmd("dequiesce", AgentAppRouter, int32(mgmt_pb.ContentType_RouterDequiesceRequestType), p)
	dequiesceCmd.Hidden = true
	routerCmd.AddCommand(dequiesceCmd)

	decommissionCmd := NewSimpleChAgentCustomCmd("decommission", AgentAppRouter, int32(mgmt_pb.ContentType_RouterDecommissionRequestType), p)
	routerCmd.AddCommand(decommissionCmd)

	return agentCmd
}

// AgentOptions contains the command line options
type AgentOptions struct {
	common.CommonOptions
	pid         uint32
	processName string
	appId       string
	appType     string
	appAlias    string
	tcpAddr     string
	timeout     time.Duration
}

func (self *AgentOptions) AddAgentOptions(cmd *cobra.Command) {
	cmd.Flags().Uint32VarP(&self.pid, "pid", "p", 0, "Process ID of host application to talk to")
	cmd.Flags().StringVarP(&self.processName, "process-name", "n", "", "Process name of host application to talk to")
	cmd.Flags().StringVarP(&self.appId, "app-id", "i", "", "Id of host application to talk to (like controller or router id)")
	cmd.Flags().StringVarP(&self.appType, "app-type", "t", "", "Type of host application to talk to (like controller or router)")
	cmd.Flags().StringVarP(&self.appAlias, "app-alias", "a", "", "Alias of host application to talk to (specified in host application)")
	cmd.Flags().StringVar(&self.tcpAddr, "tcp-addr", "", "Type of host application to talk to (like controller or router)")
	cmd.Flags().DurationVar(&self.timeout, "timeout", 5*time.Second, "Operation timeout")
}

func (self *AgentOptions) GetProcess() (*agent.Process, error) {
	procList, err := agent.GetGopsProcesses()
	if err != nil {
		return nil, err
	}
	var result []*agent.Process
	for _, p := range procList {
		if !p.Contactable {
			continue
		}
		if self.Cmd.Flags().Changed("pid") && p.Pid != int(self.pid) {
			continue
		}
		if self.Cmd.Flags().Changed("process-name") && p.Executable != self.processName {
			continue
		}
		if self.Cmd.Flags().Changed("app-id") && p.AppId != self.appId {
			continue
		}
		if self.Cmd.Flags().Changed("app-type") && p.AppType != self.appType {
			continue
		}
		if self.Cmd.Flags().Changed("app-alias") && p.AppAlias != self.appAlias {
			continue
		}

		result = append(result, p)
	}

	if len(result) == 0 {
		return nil, errors.New("no processes found matching filter, use 'ziti agent list' to list candidates")
	}

	if len(result) > 1 {
		var pids []string
		for _, r := range result {
			pids = append(pids, fmt.Sprintf("%v", r.Pid))
		}
		return nil, errors.Errorf("too many processes found matching filter: [%v] use 'ziti agent list' for more information", strings.Join(pids, " "))
	}

	return result[0], nil
}

func (self *AgentOptions) MakeChannelRequest(appId byte, f func(ch channel.Channel) error) error {
	return self.MakeRequest(agent.CustomOpAsync, []byte{appId}, connToChannelMapper(f))
}

func (self *AgentOptions) MakeRequest(signal byte, params []byte, f func(c net.Conn) error) error {
	if self.Cmd.Flags().Changed("tcp-addr") {
		conn, err := net.Dial("tcp", self.tcpAddr)
		if err != nil {
			return err
		}
		return agent.MakeRequestToConn(conn, signal, params, f)
	}

	p, err := self.GetProcess()
	if err != nil {
		return err
	}
	return agent.MakeProcessRequest(p, signal, params, f)
}

func (self *AgentOptions) CopyToWriter(out io.Writer) func(conn net.Conn) error {
	return func(conn net.Conn) error {
		_, err := io.Copy(out, conn)
		return err
	}
}

func (self *AgentOptions) RunCopyOut(op byte, params []byte, out io.Writer) error {
	if self.Cmd.Flags().Changed("timeout") {
		time.AfterFunc(self.timeout, func() {
			fmt.Println("operation timed out")
			os.Exit(-1)
		})
	}

	return self.MakeRequest(op, params, self.CopyToWriter(out))
}

func NewAgentChannel(conn net.Conn) (channel.Channel, error) {
	options := channel.DefaultOptions()
	options.ConnectTimeout = time.Second
	dialer := channel.NewExistingConnDialer(&identity.TokenId{Token: "agent"}, conn, nil)
	return channel.NewChannel("agent", dialer, nil, options)
}

func MakeAgentChannelRequest(addr string, appId byte, f func(ch channel.Channel) error) error {
	return agent.MakeRequestF(addr, agent.CustomOpAsync, []byte{appId}, connToChannelMapper(f))
}

func connToChannelMapper(f func(ch channel.Channel) error) func(conn net.Conn) error {
	return func(conn net.Conn) error {
		ch, err := NewAgentChannel(conn)
		if err != nil {
			return err
		}
		return f(ch)
	}
}
