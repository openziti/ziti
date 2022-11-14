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
	"github.com/openziti/identity"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/spf13/cobra"
	"net"
	"time"
)

type SimpleChAgentCmdOptions struct {
	AgentOptions
	requestType int32
}

func NewSimpleChAgentCustomCmd(name string, appId AgentAppId, op int32, p common.OptionsProvider) *cobra.Command {
	options := &SimpleChAgentCmdOptions{
		AgentOptions: AgentOptions{
			CommonOptions: p(),
		},
		requestType: op,
	}

	cmd := &cobra.Command{
		Args: cobra.MaximumNArgs(1),
		Use:  name + " <optional-target> ",
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run(appId)
			cmdhelper.CheckErr(err)
		},
	}

	return cmd
}

// Run implements the command
func (o *SimpleChAgentCmdOptions) Run(appId AgentAppId) error {
	addr, err := agent.ParseGopsAddress(o.Args)
	if err != nil {
		return err
	}

	return agent.MakeRequestF(addr, agent.CustomOpAsync, []byte{byte(appId)}, o.makeRequest)
}

func (o *SimpleChAgentCmdOptions) makeRequest(conn net.Conn) error {
	options := channel.DefaultOptions()
	options.ConnectTimeout = time.Second
	dialer := channel.NewExistingConnDialer(&identity.TokenId{Token: "agent"}, conn, nil)
	ch, err := channel.NewChannel("agent", dialer, nil, options)
	if err != nil {
		return err
	}

	msg := channel.NewMessage(o.requestType, nil)
	reply, err := msg.WithTimeout(5 * time.Second).SendForReply(ch)
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
	} else {
		fmt.Printf("unexpected response type %v\n", reply.ContentType)
	}
	return nil
}
