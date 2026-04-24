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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/v2/common/agentid"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/ziti/cmd/common"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/proto"
)

// NewAgentInspectCmd creates an inspect command that works against any ziti component
// (controller, router, or tunnel) by sending agentid.AppIdAny.
func NewAgentInspectCmd(p common.OptionsProvider) *cobra.Command {
	action := &agentInspectAction{
		AgentOptions: AgentOptions{
			CommonOptions: p(),
		},
	}

	cmd := &cobra.Command{
		Use:   "inspect <value> [values...]",
		Short: "Inspect local runtime state of the target process",
		Long: `Sends an inspect request directly to the target process via the agent IPC channel.
This inspects only the local process, unlike 'ziti fabric inspect' which fans out through the controller.

Common inspect keys for routers:
  stackdump, links, config, metrics, sdk-terminators, ert-terminators,
  router-circuits, router-data-model, router-controllers

Common inspect keys for controllers:
  stackdump, config, metrics, connected-routers, connected-peers,
  cluster-config, router-messaging, terminator-costs, data-model-index

Common inspect keys for tunnelers:
  stackdump, sdk`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			action.Cmd = cmd
			action.Args = args
			return action.Run()
		},
	}

	action.AddAgentOptions(cmd)
	return cmd
}

type agentInspectAction struct {
	AgentOptions
}

func (self *agentInspectAction) Run() error {
	return self.MakeChannelRequest(agentid.AppIdAny, self.makeRequest)
}

func (self *agentInspectAction) makeRequest(ch channel.Channel) error {
	request := &ctrl_pb.InspectRequest{
		RequestedValues: self.Args,
	}

	body, err := proto.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal inspect request: %w", err)
	}

	msg := channel.NewMessage(int32(ctrl_pb.ContentType_InspectRequestType), body)
	reply, err := msg.WithTimeout(self.timeout).SendForReply(ch)
	if err != nil {
		return fmt.Errorf("failed to send inspect request: %w", err)
	}

	response := &ctrl_pb.InspectResponse{}
	if err := proto.Unmarshal(reply.Body, response); err != nil {
		return fmt.Errorf("failed to unmarshal inspect response: %w", err)
	}

	if !response.Success {
		fmt.Println("Errors:")
		for _, e := range response.Errors {
			fmt.Printf("  %s\n", e)
		}
		return nil
	}

	for i, val := range response.Values {
		if i > 0 {
			fmt.Println()
		}
		fmt.Print(color.New(color.FgGreen, color.Bold).Sprintf("%s\n", val.Name))
		prettyPrintValue(val.Value)
	}

	return nil
}

func prettyPrintValue(val string) {
	// If it looks like JSON, pretty-print it
	val = strings.TrimSpace(val)
	if strings.HasPrefix(val, "{") || strings.HasPrefix(val, "[") {
		var parsed interface{}
		if err := json.Unmarshal([]byte(val), &parsed); err == nil {
			if pretty, err := json.MarshalIndent(parsed, "", "  "); err == nil {
				fmt.Println(string(pretty))
				return
			}
		}
	}
	// Otherwise print as-is
	fmt.Println(val)
}
