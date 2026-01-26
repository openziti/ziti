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
	"math"
	"strconv"

	"github.com/openziti/ziti/v2/ziti/cmd/common"
	"github.com/openziti/ziti/v2/ziti/internal/log"
	"github.com/pkg/errors"

	"github.com/shirou/gopsutil/v3/process"
	"github.com/spf13/cobra"
)

type AgentGetOptionsAction struct {
	AgentOptions
	Pid string
}

func NewGetCmd(p common.OptionsProvider) *cobra.Command {
	action := &AgentGetOptionsAction{
		AgentOptions: AgentOptions{
			CommonOptions: p(),
		},
	}

	cmd := &cobra.Command{
		Use:   "get",
		Short: "Returns information about the target process",
		RunE: func(cmd *cobra.Command, args []string) error {
			action.Cmd = cmd
			action.Args = args
			return action.Run()
		},
	}

	cmd.Flags().StringVarP(&action.Pid, "pid", "", "", "pid (target of sub-cmd)")

	return cmd
}

// Run implements the command
func (self *AgentGetOptionsAction) Run() error {
	pid, err := strconv.Atoi(self.Args[0])
	if err != nil {
		return err
	}

	if pid >= 0 && pid < math.MaxInt32 {
		processInfo(int32(pid))
		return nil
	}

	return errors.Errorf("invalid pid [%v]", self.Args[0])
}

func processInfo(pid int32) {
	p, err := process.NewProcess(pid)
	if err != nil {
		log.Fatalf("Cannot read process info: %v", err)
	}
	if v, err := p.Parent(); err == nil {
		fmt.Printf("parent PID:\t%v\n", v.Pid)
	}
	if v, err := p.NumThreads(); err == nil {
		fmt.Printf("threads:\t%v\n", v)
	}
	if v, err := p.MemoryPercent(); err == nil {
		fmt.Printf("memory usage:\t%.3f%%\n", v)
	}
	if v, err := p.CPUPercent(); err == nil {
		fmt.Printf("cpu usage:\t%.3f%%\n", v)
	}
	if v, err := p.Username(); err == nil {
		fmt.Printf("username:\t%v\n", v)
	}
	if v, err := p.Cmdline(); err == nil {
		fmt.Printf("cmd+args:\t%v\n", v)
	}
	if v, err := p.Connections(); err == nil {
		if len(v) > 0 {
			for _, conn := range v {
				fmt.Printf("local/remote:\t%v:%v <-> %v:%v (%v)\n",
					conn.Laddr.IP, conn.Laddr.Port, conn.Raddr.IP, conn.Raddr.Port, conn.Status)
			}
		}
	}
}
