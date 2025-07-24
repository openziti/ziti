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

type AgentDumpHeapAction struct {
	AgentOptions
}

func NewDumpHeapCmd(p common.OptionsProvider) *cobra.Command {
	action := &AgentDumpHeapAction{
		AgentOptions: AgentOptions{
			CommonOptions: p(),
		},
	}

	cmd := &cobra.Command{
		Use:   "dump-heap <dump file path>",
		Short: "Dumps the heap to the specified file path",
		Args:  cobra.ExactArgs(1),
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
func (self *AgentDumpHeapAction) Run() error {
	if self.Cmd.Flags().Changed("timeout") {
		time.AfterFunc(self.timeout, func() {
			fmt.Println("operation timed out")
			os.Exit(-1)
		})
	}

	outputFileName := self.Args[0]

	lenBuf := make([]byte, 8)
	lenLen := binary.PutVarint(lenBuf, int64(len(outputFileName)))
	buf := &bytes.Buffer{}
	buf.Write(lenBuf[:lenLen])
	buf.Write([]byte(outputFileName))
	return self.MakeRequest(agent.HeapDump, buf.Bytes(), self.CopyToWriter(os.Stdout))
}
