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
	"github.com/openziti/agent"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/spf13/cobra"
	"os"
)

type AgentClearChannelLogLevelAction struct {
	AgentOptions
}

func NewClearChannelLogLevelCmd(p common.OptionsProvider) *cobra.Command {
	options := &AgentClearChannelLogLevelAction{
		AgentOptions: AgentOptions{
			CommonOptions: p(),
		},
	}

	cmd := &cobra.Command{
		Use:   "clear-channel-log-level target channel",
		Short: "Clears a channel-specific log level in the target application",
		Args:  cobra.MinimumNArgs(1),
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
func (self *AgentClearChannelLogLevelAction) Run() error {
	var addr string
	var err error
	var channelArg string
	if len(self.Args) == 1 {
		addr, err = agent.ParseGopsAddress(nil)
		channelArg = self.Args[0]
	} else {
		addr, err = agent.ParseGopsAddress(self.Args)
		channelArg = self.Args[1]
	}

	if err != nil {
		return err
	}

	lenBuf := make([]byte, 8)
	lenLen := binary.PutVarint(lenBuf, int64(len(channelArg)))
	buf := &bytes.Buffer{}
	buf.Write(lenBuf[:lenLen])
	buf.Write([]byte(channelArg))

	return agent.MakeRequest(addr, agent.ClearChannelLogLevel, buf.Bytes(), os.Stdout)
}
