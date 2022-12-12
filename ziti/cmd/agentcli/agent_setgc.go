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
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/spf13/cobra"
	"os"
	"strconv"
)

type AgentSetGcAction struct {
	AgentOptions
}

func NewSetGcCmd(p common.OptionsProvider) *cobra.Command {
	action := &AgentSetGcAction{
		AgentOptions: AgentOptions{
			CommonOptions: p(),
		},
	}

	cmd := &cobra.Command{
		Use:   "setgc target gc-percentage",
		Short: "Sets the GC percentage in the target application",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			action.Cmd = cmd
			action.Args = args
			err := action.Run()
			cmdhelper.CheckErr(err)
		},
	}

	return cmd
}

// Run implements the command
func (o *AgentSetGcAction) Run() error {
	var addr string
	var err error
	var pctArg string
	if len(o.Args) == 1 {
		addr, err = agent.ParseGopsAddress(nil)
		pctArg = o.Args[0]
	} else {
		addr, err = agent.ParseGopsAddress(o.Args)
		pctArg = o.Args[1]
	}

	if err != nil {
		return err
	}

	perc, err := strconv.ParseInt(pctArg, 10, strconv.IntSize)
	if err != nil {
		return err
	}
	buf := make([]byte, binary.MaxVarintLen64)
	binary.PutVarint(buf, perc)

	return agent.MakeRequest(addr, agent.SetGCPercent, buf, os.Stdout)
}
