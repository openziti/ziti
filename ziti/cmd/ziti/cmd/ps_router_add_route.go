/*
	Copyright NetFoundry, Inc.

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

package cmd

import (
	"encoding/binary"
	"github.com/golang/protobuf/proto"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/router"
	"github.com/openziti/foundation/agent"
	cmdutil "github.com/openziti/ziti/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/spf13/cobra"
	"io"
	"os"
)

// PsRouteOptions the options for the create spring command
type PsRouteOptions struct {
	PsOptions
	CtrlListener string
}

// NewCmdPsRoute creates a command object for the "create" command
func NewCmdPsRoute(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &PsRouteOptions{
		PsOptions: PsOptions{
			CommonOptions: CommonOptions{
				Factory: f,
				Out:     out,
				Err:     errOut,
			},
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

	options.addCommonFlags(cmd)

	return cmd
}

// Run implements the command
func (o *PsRouteOptions) Run() error {
	var addr string
	var err error

	offset := 0
	if len(o.Args) == 4 {
		addr, err = agent.ParseGopsAddress(o.Args)
		if err != nil {
			return err
		}
		offset = 1
	}

	route := &ctrl_pb.Route{
		SessionId: o.Args[offset],
		Forwards: []*ctrl_pb.Route_Forward{
			{
				SrcAddress: o.Args[offset+1],
				DstAddress: o.Args[offset+2],
			},
		},
	}

	buf, err := proto.Marshal(route)
	if err != nil {
		return err
	}

	fullBuf := make([]byte, len(buf)+5)
	fullBuf[0] = router.UpdateRoute

	sizeBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(sizeBuf, uint32(len(buf)))

	copy(fullBuf[1:], sizeBuf)
	copy(fullBuf[5:], buf)

	return agent.MakeRequest(addr, agent.CustomOp, fullBuf, os.Stdout)
}
