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
	"github.com/openziti/agent"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"os"
	"strings"
)

type AgentSetLogLevelAction struct {
	AgentOptions
}

func NewSetLogLevelCmd(p common.OptionsProvider) *cobra.Command {
	action := &AgentSetLogLevelAction{
		AgentOptions: AgentOptions{
			CommonOptions: p(),
		},
	}

	cmd := &cobra.Command{
		Use:   "set-log-level target log-level (panic, fatal, error, warn, info, debug, trace)",
		Short: "Sets the global logrus logging level in the target application",
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
func (self *AgentSetLogLevelAction) Run() error {
	var addr string
	var err error
	var levelArg string
	if len(self.Args) == 1 {
		addr, err = agent.ParseGopsAddress(nil)
		levelArg = self.Args[0]
	} else {
		addr, err = agent.ParseGopsAddress(self.Args)
		levelArg = self.Args[1]
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

	buf := []byte{byte(level)}
	return agent.MakeRequest(addr, agent.SetLogLevel, buf, os.Stdout)
}
