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
	"github.com/openziti/channel/v2"
	"github.com/openziti/ziti/common/pb/mgmt_pb"
	"github.com/openziti/ziti/controller"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/spf13/cobra"
)

type AgentClusterRemoveAction struct {
	AgentOptions
}

func NewAgentClusterRemove(p common.OptionsProvider) *cobra.Command {
	action := AgentClusterRemoveAction{
		AgentOptions: AgentOptions{
			CommonOptions: p(),
		},
	}

	cmd := &cobra.Command{
		Args:  cobra.ExactArgs(1),
		Use:   "remove <node id>",
		Short: "removes a node from the controller cluster",
		Run: func(cmd *cobra.Command, args []string) {
			action.Cmd = cmd
			action.Args = args
			err := action.MakeChannelRequest(byte(AgentAppController), action.makeRequest)
			cmdhelper.CheckErr(err)
		},
	}
	action.AddAgentOptions(cmd)
	return cmd
}

func (o *AgentClusterRemoveAction) makeRequest(ch channel.Channel) error {
	msg := channel.NewMessage(int32(mgmt_pb.ContentType_RaftRemovePeerRequestType), nil)

	msg.PutStringHeader(controller.AgentIdHeader, o.Args[0])
	reply, err := msg.WithTimeout(o.timeout).SendForReply(ch)

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
