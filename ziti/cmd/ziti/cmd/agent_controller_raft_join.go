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
	"fmt"
	"github.com/openziti/channel/v2"
	"github.com/openziti/fabric/controller"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/openziti/agent"
	"github.com/openziti/identity"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/agentcli"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/spf13/cobra"
	"net"
	"time"
)

type AgentCtrlRaftJoinOptions struct {
	agentcli.AgentOptions
	Voter bool
}

func NewAgentCtrlRaftJoin(p common.OptionsProvider) *cobra.Command {
	options := &AgentCtrlRaftJoinOptions{
		AgentOptions: agentcli.AgentOptions{
			CommonOptions: p(),
		},
	}

	cmd := &cobra.Command{
		Args: cobra.RangeArgs(2, 3),
		Use:  "raft-join <optional-target> <id> <addr>",
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}

	cmd.Flags().BoolVar(&options.Voter, "voter", true, "Is this member a voting member")

	return cmd
}

// Run implements the command
func (o *AgentCtrlRaftJoinOptions) Run() error {
	var addr string
	var err error

	if len(o.Args) == 3 {
		addr, err = agent.ParseGopsAddress(o.Args)
		if err != nil {
			return err
		}
	}

	return agent.MakeRequestF(addr, agent.CustomOpAsync, []byte{byte(AgentAppController)}, o.makeRequest)
}

func (o *AgentCtrlRaftJoinOptions) makeRequest(conn net.Conn) error {
	options := channel.DefaultOptions()
	options.ConnectTimeout = time.Second
	dialer := channel.NewExistingConnDialer(&identity.TokenId{Token: "agent"}, conn, nil)
	ch, err := channel.NewChannel("agent", dialer, nil, options)
	if err != nil {
		return err
	}

	offset := 0
	if len(o.Args) == 3 {
		offset = 1
	}

	msg := channel.NewMessage(int32(mgmt_pb.ContentType_RaftJoinRequestType), nil)
	msg.PutStringHeader(controller.AgentIdHeader, o.Args[offset])
	msg.PutStringHeader(controller.AgentAddrHeader, o.Args[offset+1])
	msg.PutBoolHeader(controller.AgentIsVoterHeader, o.Voter)

	reply, err := msg.WithTimeout(5 * time.Second).SendForReply(ch)
	if err != nil {
		return err
	}
	result := channel.UnmarshalResult(reply)
	if result.Success {
		fmt.Println("success")
	} else {
		fmt.Printf("error: %v\n", result.Message)
	}
	return nil
}
