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
	"github.com/openziti/agent"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/spf13/cobra"
	"os"
	"time"
)

type AgentClearChannelLogLevelAction struct {
	AgentOptions
}

func NewClearChannelLogLevelCmd(p common.OptionsProvider) *cobra.Command {
	action := &AgentClearChannelLogLevelAction{
		AgentOptions: AgentOptions{
			CommonOptions: p(),
		},
	}

	cmd := &cobra.Command{
		Use:   "clear-channel-log-level target channel",
		Short: "Clears a channel-specific log level in the target application",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			action.Cmd = cmd
			action.Args = args
			err := action.Run()
			cmdhelper.CheckErr(err)
		},
	}

	action.AddAgentOptions(cmd)

	return cmd
}

// Run implements the command
func (self *AgentClearChannelLogLevelAction) Run() error {
	if self.Cmd.Flags().Changed("timeout") {
		time.AfterFunc(self.timeout, func() {
			fmt.Println("operation timed out")
			os.Exit(-1)
		})
	}

	var channelArg string

	if len(self.Args) == 1 {
		channelArg = self.Args[0]
	} else {
		channelArg = self.Args[1]
	}

	lenBuf := make([]byte, 8)
	lenLen := binary.PutVarint(lenBuf, int64(len(channelArg)))
	buf := &bytes.Buffer{}
	buf.Write(lenBuf[:lenLen])
	buf.Write([]byte(channelArg))

	if len(self.Args) == 1 {
		return self.MakeRequest(agent.ClearChannelLogLevel, buf.Bytes(), self.CopyToWriter(os.Stdout))
	}

	addr, err := agent.ParseGopsAddress(self.Args)
	if err != nil {
		return err
	}
	return agent.MakeRequest(addr, agent.ClearChannelLogLevel, buf.Bytes(), os.Stdout)
}
