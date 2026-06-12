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
	"bytes"
	"encoding/binary"
	"fmt"
	"os"

	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/v2/common/agent"
	"github.com/openziti/ziti/v2/ziti/cmd/common"
	"github.com/spf13/cobra"
)

type AgentSetChannelLogLevelAction struct {
	AgentOptions
}

func NewSetChannelLogLevelCmd(p common.OptionsProvider) *cobra.Command {
	action := &AgentSetChannelLogLevelAction{
		AgentOptions: AgentOptions{
			CommonOptions: p(),
		},
	}

	cmd := &cobra.Command{
		Use:   "set-channel-log-level channel log-level (panic, fatal, error, warn, info, debug, trace)",
		Short: "Sets a channel-specific log level in the target application",
		Args:  cobra.ExactArgs(2),
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
func (self *AgentSetChannelLogLevelAction) Run() error {
	channelArg := self.Args[0]
	level, err := agent.ParseLogLevel(self.Args[1])
	if err != nil {
		return err
	}

	if self.HasAgentCapability(agent.CapabilityLoggingSlogLevels) {
		return self.MakeChannelRequest(byte(AgentAppAny), func(ch channel.Channel) error {
			msg, err := agent.SendSetChannelLogLevelV2(ch, channelArg, level, self.timeout)
			if err != nil {
				return err
			}
			fmt.Println(msg)
			return nil
		})
	}

	lenBuf := make([]byte, 8)
	lenLen := binary.PutVarint(lenBuf, int64(len(channelArg)))
	buf := &bytes.Buffer{}
	buf.Write(lenBuf[:lenLen])
	buf.Write([]byte(channelArg))
	buf.WriteByte(byte(level))

	return self.MakeRequest(agent.SetChannelLogLevel, buf.Bytes(), self.CopyToWriter(os.Stdout))
}
