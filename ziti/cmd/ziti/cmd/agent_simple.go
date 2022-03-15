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
	"github.com/openziti/foundation/agent"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/spf13/cobra"
	"io"
	"os"
)

type SimpleAgentCmdOptions struct {
	PsOptions
}

func NewSimpleAgentCustomCmd(name string, appId AgentAppId, op byte, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &SimpleAgentCmdOptions{
		PsOptions: PsOptions{
			CommonOptions: CommonOptions{
				Out: out,
				Err: errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Args: cobra.MaximumNArgs(1),
		Use:  name + " <optional-target> ",
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run(appId, op)
			cmdhelper.CheckErr(err)
		},
	}

	options.addCommonFlags(cmd)

	return cmd
}

// Run implements the command
func (o *SimpleAgentCmdOptions) Run(appId AgentAppId, op byte) error {
	addr, err := agent.ParseGopsAddress(o.Args)
	if err != nil {
		return err
	}

	buf := []byte{byte(appId), op}

	return agent.MakeRequest(addr, agent.CustomOp, buf, os.Stdout)
}
