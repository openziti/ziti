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

package agentcli

import (
	"fmt"
	"github.com/openziti/agent"
	"github.com/openziti/channel/v2"
	"github.com/openziti/fabric/controller"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/openziti/identity"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/spf13/cobra"
	"net"
	"time"
)

type AgentCtrlRaftJoinOptions struct {
	AgentOptions
	Voter    bool
	MemberId string
}

func NewAgentCtrlRaftJoin(p common.OptionsProvider) *cobra.Command {
	options := &AgentCtrlRaftJoinOptions{
		AgentOptions: AgentOptions{
			CommonOptions: p(),
		},
	}

	cmd := &cobra.Command{
		Args: cobra.RangeArgs(1, 2),
		Use:  "raft-join <optional-target> <addr>",
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}

	cmd.Flags().BoolVar(&options.Voter, "voter", true, "Is this member a voting member")
	cmd.Flags().StringVar(&options.MemberId, "id", "", "The member id. If not provided, it will be looked up")

	return cmd
}

// Run implements the command
func (o *AgentCtrlRaftJoinOptions) Run() error {
	var addr string
	var err error

	if len(o.Args) == 2 {
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
	if len(o.Args) == 2 {
		offset = 1
	}

	msg := channel.NewMessage(int32(mgmt_pb.ContentType_RaftJoinRequestType), nil)
	msg.PutStringHeader(controller.AgentAddrHeader, o.Args[offset])
	msg.PutBoolHeader(controller.AgentIsVoterHeader, o.Voter)

	if o.MemberId != "" {
		msg.PutStringHeader(controller.AgentIdHeader, o.MemberId)
	}

	reply, err := msg.WithTimeout(5 * time.Second).SendForReply(ch)
	if err != nil {
		return err
	}
	result := channel.UnmarshalResult(reply)
	if result.Success {
		fmt.Println(result.Message)
	} else {
		fmt.Printf("error: %v\n", result.Message)
	}
	return nil
}
