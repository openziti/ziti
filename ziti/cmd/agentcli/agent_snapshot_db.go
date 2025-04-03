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
	"errors"
	"fmt"
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/common/pb/mgmt_pb"
	"github.com/openziti/ziti/controller"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/spf13/cobra"
)

type AgentSnapshoptDbAction struct {
	AgentOptions
}

func NewAgentSnapshotDb(p common.OptionsProvider) *cobra.Command {
	action := &AgentSnapshoptDbAction{
		AgentOptions: AgentOptions{
			CommonOptions: p(),
		},
	}

	cmd := &cobra.Command{
		Args: cobra.ExactArgs(1),
		Use:  "snapshot-db <snapshot path>",
		RunE: func(cmd *cobra.Command, args []string) error {
			action.Cmd = cmd
			action.Args = args
			return action.MakeChannelRequest(byte(AgentAppController), action.makeRequest)
		},
	}
	cmd.Args = cobra.MaximumNArgs(1)
	action.AddAgentOptions(cmd)

	return cmd
}

func (self *AgentSnapshoptDbAction) makeRequest(ch channel.Channel) error {
	msg := channel.NewMessage(int32(mgmt_pb.ContentType_SnapshotDbRequestType), nil)
	if len(self.Args) > 0 {
		msg.PutStringHeader(controller.AgentSnapshotFileName, self.Args[0])
	}

	reply, err := msg.WithTimeout(self.timeout).SendForReply(ch)
	if err != nil {
		return err
	}
	result := channel.UnmarshalResult(reply)
	if result.Success {
		fmt.Println(result.Message)
	} else {
		return errors.New(result.Message)
	}
	return nil
}
