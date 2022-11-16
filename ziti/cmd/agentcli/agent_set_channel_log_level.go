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
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"os"
	"strings"
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
		Use:   "set-channel-log-level target channel log-level (panic, fatal, error, warn, info, debug, trace)",
		Short: "Sets a channel-specific log level in the target application",
		Args:  cobra.MinimumNArgs(2),
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
func (self *AgentSetChannelLogLevelAction) Run() error {
	var addr string
	var err error
	var channelArg string
	var levelArg string
	if len(self.Args) == 2 {
		addr, err = agent.ParseGopsAddress(nil)
		channelArg = self.Args[0]
		levelArg = self.Args[1]
	} else {
		addr, err = agent.ParseGopsAddress(self.Args)
		channelArg = self.Args[1]
		levelArg = self.Args[2]
	}

	if err != nil {
		return err
	}

	var level logrus.Level
	var found bool
	for _, l := range logrus.AllLevels {
		if strings.EqualFold(l.String(), levelArg) {
			level = l
			found = true
		}
	}

	if !found {
		return errors.Errorf("invalid log level %v", levelArg)
	}

	lenBuf := make([]byte, 8)
	lenLen := binary.PutVarint(lenBuf, int64(len(channelArg)))
	buf := &bytes.Buffer{}
	buf.Write(lenBuf[:lenLen])
	buf.Write([]byte(channelArg))
	buf.WriteByte(byte(level))

	return agent.MakeRequest(addr, agent.SetChannelLogLevel, buf.Bytes(), os.Stdout)
}
