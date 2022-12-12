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
	"encoding/binary"
	"github.com/openziti/agent"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/router"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/proto"
	"os"
)

type AgentRouteAction struct {
	AgentOptions
	CtrlListener string
}

func NewRouteCmd(p common.OptionsProvider) *cobra.Command {
	options := &AgentRouteAction{
		AgentOptions: AgentOptions{
			CommonOptions: p(),
		},
	}

	cmd := &cobra.Command{
		Args: cobra.RangeArgs(3, 4),
		Use:  "route <optional-target> <session id> <source-address> <destination-address>",
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
func (self *AgentRouteAction) Run() error {
	var addr string
	var err error

	offset := 0
	if len(self.Args) == 4 {
		addr, err = agent.ParseGopsAddress(self.Args)
		if err != nil {
			return err
		}
		offset = 1
	}

	route := &ctrl_pb.Route{
		CircuitId: self.Args[offset],
		Forwards: []*ctrl_pb.Route_Forward{
			{
				SrcAddress: self.Args[offset+1],
				DstAddress: self.Args[offset+2],
			},
		},
	}

	buf, err := proto.Marshal(route)
	if err != nil {
		return err
	}

	fullBuf := make([]byte, len(buf)+6)
	fullBuf[0] = byte(AgentAppRouter)
	fullBuf[1] = router.UpdateRoute

	sizeBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(sizeBuf, uint32(len(buf)))

	copy(fullBuf[2:], sizeBuf)
	copy(fullBuf[6:], buf)

	return agent.MakeRequest(addr, agent.CustomOp, fullBuf, os.Stdout)
}
