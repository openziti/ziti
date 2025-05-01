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
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/common/pb/mgmt_pb"
	"github.com/openziti/ziti/controller"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/spf13/cobra"
	"os"
)

type AgentClusterAddAction struct {
	AgentOptions
	Voter bool
}

func NewAgentClusterAdd(p common.OptionsProvider) *cobra.Command {
	action := &AgentClusterAddAction{
		AgentOptions: AgentOptions{
			CommonOptions: p(),
		},
	}

	cmd := &cobra.Command{
		Args:  cobra.ExactArgs(1),
		Use:   "add <addr>",
		Short: "adds a node to the controller cluster",
		Run: func(cmd *cobra.Command, args []string) {
			action.Cmd = cmd
			action.Args = args
			if err := action.MakeChannelRequest(byte(AgentAppController), action.makeRequest); err != nil {
				_, _ = fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		},
	}

	action.AddAgentOptions(cmd)
	cmd.Flags().BoolVar(&action.Voter, "voter", true, "Is this member a voting member")

	return cmd
}

func (self *AgentClusterAddAction) makeRequest(ch channel.Channel) error {
	msg := channel.NewMessage(int32(mgmt_pb.ContentType_RaftAddPeerRequestType), nil)
	msg.PutStringHeader(controller.AgentAddrHeader, self.Args[0])
	msg.PutBoolHeader(controller.AgentIsVoterHeader, self.Voter)

	reply, err := msg.WithTimeout(self.timeout).SendForReply(ch)
	if err != nil {
		return err
	}
	result := channel.UnmarshalResult(reply)
	if !result.Success {
		return fmt.Errorf("cluster add failed: %s", result.Message)
	}
	fmt.Println(result.Message)
	return nil
}
