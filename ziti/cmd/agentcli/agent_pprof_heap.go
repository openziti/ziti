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
	"io"
	"os"
	"time"
)

type AgentPprofHeapAction struct {
	AgentOptions
	outputFile string
}

func NewPprofHeapCmd(p common.OptionsProvider) *cobra.Command {
	action := &AgentPprofHeapAction{
		AgentOptions: AgentOptions{
			CommonOptions: p(),
		},
	}

	cmd := &cobra.Command{
		Use:   "pprof-heap",
		Short: "Returns a memory heap pprof of the target application",
		Run: func(cmd *cobra.Command, args []string) {
			action.Cmd = cmd
			action.Args = args
			err := action.Run()
			cmdhelper.CheckErr(err)
		},
	}

	action.AddAgentOptions(cmd)
	cmd.Flags().StringVarP(&action.outputFile, "output-file", "o", "", "Output file for pprof")

	return cmd
}

// Run implements the command
func (self *AgentPprofHeapAction) Run() error {
	if self.Cmd.Flags().Changed("timeout") {
		time.AfterFunc(self.timeout, func() {
			fmt.Println("operation timed out")
			os.Exit(-1)
		})
	}

	if len(self.Args) == 0 {
		var out io.WriteCloser = os.Stdout
		var err error
		if self.outputFile != "" {
			out, err = os.Create(self.outputFile)
			if err != nil {
				return err
			}
			defer out.Close()
		}
		return self.RunCopyOut(agent.HeapProfile, nil, out)
	}

	addr, err := agent.ParseGopsAddress(self.Args)
	if err != nil {
		return err
	}

	var out io.WriteCloser = os.Stdout
	if len(self.Args) > 1 {
		out, err = os.Create(self.Args[1])
		if err != nil {
			return err
		}
		defer out.Close()
	}

	return agent.MakeRequest(addr, agent.HeapProfile, nil, out)
}
