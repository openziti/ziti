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
	"google.golang.org/protobuf/proto"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/openziti/channel/v2"
	"github.com/openziti/ziti/common/pb/mgmt_pb"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/spf13/cobra"
)

type AgentClusterListAction struct {
	AgentOptions
}

func NewAgentClusterList(p common.OptionsProvider) *cobra.Command {
	action := &AgentClusterListAction{
		AgentOptions: AgentOptions{
			CommonOptions: p(),
		},
	}

	cmd := &cobra.Command{
		Args:  cobra.RangeArgs(0, 1),
		Use:   "list",
		Short: "lists the nodes in the controller cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			action.Cmd = cmd
			action.Args = args
			return action.MakeChannelRequest(byte(AgentAppController), action.makeRequest)
		},
	}

	action.AddAgentOptions(cmd)

	return cmd
}

func (self *AgentClusterListAction) makeRequest(ch channel.Channel) error {
	msg := channel.NewMessage(int32(mgmt_pb.ContentType_RaftListMembersRequestType), nil)
	reply, err := msg.WithTimeout(self.timeout).SendForReply(ch)
	if err != nil {
		return err
	}
	if reply.ContentType == channel.ContentTypeResultType {
		result := channel.UnmarshalResult(reply)
		if result.Success {
			fmt.Println("success")
		} else {
			fmt.Printf("error: %v\n", result.Message)
		}
	} else if reply.ContentType == int32(mgmt_pb.ContentType_RaftListMembersResponseType) {
		resp := &mgmt_pb.RaftMemberListResponse{}
		if err = proto.Unmarshal(reply.Body, resp); err != nil {
			return err
		}
		t := table.NewWriter()
		t.SetStyle(table.StyleRounded)
		t.AppendHeader(table.Row{"Id", "Address", "Voter", "Leader", "Version", "Connected"})
		for _, m := range resp.Members {
			t.AppendRow(table.Row{m.Id, m.Addr, m.IsVoter, m.IsLeader, m.Version, m.IsConnected})
		}
		api.RenderTable(&api.Options{
			CommonOptions: self.CommonOptions,
		}, t, nil)
	}
	return nil
}
