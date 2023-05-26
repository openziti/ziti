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
	"github.com/spf13/cobra"
	"os"
)

type SimpleAgentAction struct {
	AgentOptions
}

func NewSimpleAgentCmd(name string, op byte, p common.OptionsProvider, desc string) *cobra.Command {
	action := &SimpleAgentAction{
		AgentOptions: AgentOptions{
			CommonOptions: p(),
		},
	}

	cmd := &cobra.Command{
		Args:  cobra.ExactArgs(0),
		Use:   name,
		Short: desc,
		RunE: func(cmd *cobra.Command, args []string) error {
			action.Cmd = cmd
			action.Args = args
			return action.RunCopyOut(op, nil, os.Stdout)
		},
	}

	action.AddAgentOptions(cmd)

	return cmd
}

func NewSimpleAgentCustomCmd(name string, appId AgentAppId, op byte, p common.OptionsProvider) *cobra.Command {
	action := &SimpleAgentAction{
		AgentOptions: AgentOptions{
			CommonOptions: p(),
		},
	}

	cmd := &cobra.Command{
		Args: cobra.ExactArgs(0),
		Use:  name,
		Run: func(cmd *cobra.Command, args []string) {
			action.Cmd = cmd
			action.Args = args
			err := action.Run(appId, op)
			cmdhelper.CheckErr(err)
		},
	}

	action.AddAgentOptions(cmd)

	return cmd
}

// Run implements the command
func (self *SimpleAgentAction) Run(appId AgentAppId, op byte) error {
	buf := []byte{byte(appId), op}
	return self.RunCopyOut(agent.CustomOpAsync, buf, os.Stdout)
}
