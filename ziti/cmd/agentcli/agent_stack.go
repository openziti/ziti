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
	"fmt"
	"github.com/openziti/agent"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/spf13/cobra"
	"os"
	"time"
)

type AgentStackAction struct {
	AgentOptions
	StackTimeout time.Duration
}

func NewStackCmd(p common.OptionsProvider) *cobra.Command {
	action := &AgentStackAction{
		AgentOptions: AgentOptions{
			CommonOptions: p(),
		},
	}

	cmd := &cobra.Command{
		Args:  cobra.ExactArgs(0),
		Use:   "stack",
		Short: "Emits a go-routine stack dump from the target application",
		Run: func(cmd *cobra.Command, args []string) {
			action.Cmd = cmd
			action.Args = args
			err := action.Run()
			cmdhelper.CheckErr(err)
		},
	}

	action.AddAgentOptions(cmd)
	cmd.Flags().DurationVar(&action.StackTimeout, "stack-timeout", 5*time.Second, "Timeout for stack operation (deprecated, use --timeout instead)")

	return cmd
}

// Run implements the command
func (o *AgentStackAction) Run() error {
	time.AfterFunc(o.StackTimeout, func() {
		fmt.Println("operation timed out")
		os.Exit(-1)
	})
	return o.RunCopyOut(agent.StackTrace, nil, os.Stdout)
}
