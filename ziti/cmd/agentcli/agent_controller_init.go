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
	"github.com/openziti/channel/v2/protobufs"
	"github.com/openziti/edge/pb/edge_mgmt_pb"
	"github.com/openziti/identity"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/spf13/cobra"
	"net"
	"time"
)

type AgentCtrlInitOptions struct {
	AgentOptions
}

func NewAgentCtrlInit(p common.OptionsProvider) *cobra.Command {
	options := &AgentCtrlInitOptions{
		AgentOptions: AgentOptions{
			CommonOptions: p(),
		},
	}

	cmd := &cobra.Command{
		Args: cobra.RangeArgs(3, 4),
		Use:  "init <optional-target> <username> <password> <name>",
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}

	return cmd
}

// Run implements the command
func (o *AgentCtrlInitOptions) Run() error {
	var addr string
	var err error

	if len(o.Args) == 4 {
		addr, err = agent.ParseGopsAddress(o.Args)
		if err != nil {
			return err
		}
	}

	return agent.MakeRequestF(addr, agent.CustomOpAsync, []byte{byte(AgentAppController)}, o.makeRequest)
}

func (o *AgentCtrlInitOptions) makeRequest(conn net.Conn) error {
	options := channel.DefaultOptions()
	options.ConnectTimeout = time.Second
	dialer := channel.NewExistingConnDialer(&identity.TokenId{Token: "agent"}, conn, nil)
	ch, err := channel.NewChannel("agent", dialer, nil, options)
	if err != nil {
		return err
	}

	offset := 0
	if len(o.Args) == 4 {
		offset = 1
	}

	initEdgeRequest := &edge_mgmt_pb.InitEdgeRequest{
		Username: o.Args[offset],
		Password: o.Args[offset+1],
		Name:     o.Args[offset+2],
	}

	reply, err := protobufs.MarshalTyped(initEdgeRequest).WithTimeout(5 * time.Second).SendForReply(ch)
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
