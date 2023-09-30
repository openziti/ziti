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
	"github.com/openziti/channel/v2/protobufs"
	"github.com/openziti/ziti/common/pb/edge_mgmt_pb"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/spf13/cobra"
)

type AgentCtrlInitOptions struct {
	AgentOptions
}

func NewAgentCtrlInit(p common.OptionsProvider) *cobra.Command {
	action := &AgentCtrlInitOptions{
		AgentOptions: AgentOptions{
			CommonOptions: p(),
		},
	}

	cmd := &cobra.Command{
		Args: cobra.ExactArgs(3),
		Use:  "init <username> <password> <name>",
		RunE: func(cmd *cobra.Command, args []string) error {
			action.Cmd = cmd
			action.Args = args
			return action.MakeChannelRequest(byte(AgentAppController), action.makeRequest)
		},
	}

	action.AddAgentOptions(cmd)

	return cmd
}

func (self *AgentCtrlInitOptions) makeRequest(ch channel.Channel) error {
	initEdgeRequest := &edge_mgmt_pb.InitEdgeRequest{
		Username: self.Args[0],
		Password: self.Args[1],
		Name:     self.Args[2],
	}

	reply, err := protobufs.MarshalTyped(initEdgeRequest).WithTimeout(self.timeout).SendForReply(ch)
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
