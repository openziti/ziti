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
	"os"
	"strconv"

	"github.com/openziti/agent"
	"github.com/openziti/ziti/v2/ziti/cmd/common"
	"github.com/spf13/cobra"
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
		Use:   "setgc gc-percentage",
		Short: "Sets the GC percentage in the target application",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			action.Cmd = cmd
			action.Args = args
			return action.RunWithTimeout(action.Run)
		},
	}

	action.AddAgentOptions(cmd)

	return cmd
}

// Run implements the command
func (self *AgentSetGcAction) Run() error {
	pctArg := self.Args[0]

	perc, err := strconv.ParseInt(pctArg, 10, strconv.IntSize)
	if err != nil {
		return err
	}
	buf := make([]byte, binary.MaxVarintLen64)
	binary.PutVarint(buf, perc)

	return self.MakeRequest(agent.SetGCPercent, buf, self.CopyToWriter(os.Stdout))
}
