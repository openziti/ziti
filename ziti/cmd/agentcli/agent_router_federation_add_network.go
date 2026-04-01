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
	"github.com/openziti/ziti/v2/common/pb/mgmt_pb"
	"github.com/openziti/ziti/v2/router"
	"github.com/openziti/ziti/v2/ziti/cmd/common"
	"github.com/spf13/cobra"
)

type FederationAddNetworkAgentAction struct {
	AgentOptions
	networkId uint16
}

func NewFederationAddNetworkAgentCmd(p common.OptionsProvider) *cobra.Command {
	action := &FederationAddNetworkAgentAction{
		AgentOptions: AgentOptions{
			CommonOptions: p(),
		},
	}

	cmd := &cobra.Command{
		Args: cobra.ExactArgs(1),
		Use:  "federation-add-network <jwt-path>",
		Short: "Enroll this router with a federated client network",
		RunE: func(cmd *cobra.Command, args []string) error {
			action.Cmd = cmd
			action.Args = args
			return action.Run()
		},
	}

	cmd.Flags().Uint16Var(&action.networkId, "network-id", 0, "16-bit network identifier for the client network (required)")
	_ = cmd.MarkFlagRequired("network-id")

	action.AddAgentOptions(cmd)

	return cmd
}

// Run implements the command
func (self *FederationAddNetworkAgentAction) Run() error {
	return self.MakeChannelRequest(router.AgentAppId, self.makeRequest)
}

func (self *FederationAddNetworkAgentAction) makeRequest(ch channel.Channel) error {
	jwtPath := self.Args[0]

	msg := channel.NewMessage(int32(mgmt_pb.ContentType_RouterFederationAddNetworkRequestType), []byte(jwtPath))
	msg.PutUint16Header(int32(mgmt_pb.Header_FederationNetworkId), self.networkId)

	reply, err := msg.WithTimeout(self.timeout).SendForReply(ch)
	if err != nil {
		return err
	}
	if reply.ContentType == channel.ContentTypeResultType {
		result := channel.UnmarshalResult(reply)
		if result.Success {
			fmt.Printf("success: %v\n", result.Message)
		} else {
			fmt.Printf("error: %v\n", result.Message)
		}
	} else {
		fmt.Printf("unexpected response type %v\n", reply.ContentType)
	}
	return nil
}
