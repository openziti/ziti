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
	"time"

	"github.com/openziti/channel/v2"
	"github.com/openziti/fabric/controller"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/spf13/cobra"
)

type AgentCtrlRaftLeaveAction struct {
	AgentOptions
	MemberId   string
	MemberAddr string
}

func NewAgentCtrlRaftLeave(p common.OptionsProvider) *cobra.Command {
	action := AgentCtrlRaftLeaveAction{
		AgentOptions: AgentOptions{
			CommonOptions: p(),
		},
	}

	cmd := &cobra.Command{
		Args: cobra.RangeArgs(0, 1),
		Use:  "raft-leave",
		Run: func(cmd *cobra.Command, args []string) {
			action.Cmd = cmd
			action.Args = args
			err := action.MakeChannelRequest(byte(AgentAppController), action.makeRequest)
			cmdhelper.CheckErr(err)
		},
	}
	action.AddAgentOptions(cmd)
	cmd.Flags().StringVar(&action.MemberId, "id", "", "The member id. If not provided, it will be looked up by the provided address")
	cmd.Flags().StringVar(&action.MemberAddr, "address", "", "The member address")

	return cmd
}

func (o *AgentCtrlRaftLeaveAction) makeRequest(ch channel.Channel) error {
	msg := channel.NewMessage(int32(mgmt_pb.ContentType_RaftRemoveRequestType), nil)

	if o.MemberId != "" {
		msg.PutStringHeader(controller.AgentIdHeader, o.MemberId)
	}

	if o.MemberAddr != "" {
		msg.PutStringHeader(controller.AgentAddrHeader, o.MemberAddr)
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
