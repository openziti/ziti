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
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/spf13/cobra"
)

type SimpleChAgentCmdAction struct {
	AgentOptions
	requestType int32
}

func NewSimpleChAgentCustomCmd(name string, appId AgentAppId, op int32, p common.OptionsProvider) *cobra.Command {
	action := &SimpleChAgentCmdAction{
		AgentOptions: AgentOptions{
			CommonOptions: p(),
		},
		requestType: op,
	}

	cmd := &cobra.Command{
		Args: cobra.ExactArgs(0),
		Use:  name,
		RunE: func(cmd *cobra.Command, args []string) error {
			action.Cmd = cmd
			action.Args = args
			return action.Run(appId)
		},
	}

	action.AddAgentOptions(cmd)

	return cmd
}

// Run implements the command
func (self *SimpleChAgentCmdAction) Run(appId AgentAppId) error {
	return self.MakeChannelRequest(byte(appId), self.makeRequest)
}

func (self *SimpleChAgentCmdAction) makeRequest(ch channel.Channel) error {
	msg := channel.NewMessage(self.requestType, nil)
	reply, err := msg.WithTimeout(self.timeout).SendForReply(ch)
	if err != nil {
		return err
	}
	if reply.ContentType == channel.ContentTypeResultType {
		result := channel.UnmarshalResult(reply)
		if result.Success {
			if len(result.Message) != 0 {
				fmt.Print(result.Message)
			} else {
				fmt.Println("success")
			}
		} else {
			fmt.Printf("error: %v\n", result.Message)
		}
	} else {
		fmt.Printf("unexpected response type %v\n", reply.ContentType)
	}
	return nil
}
