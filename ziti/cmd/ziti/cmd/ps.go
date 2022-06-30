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

package cmd

import (
	"bytes"
	"fmt"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/agentcli"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/common"
	"github.com/spf13/cobra"
	"os"
	"regexp"
	"strconv"
	"strings"

	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/goprocess"
)

type PsAction struct {
	agentcli.AgentOptions
}

// NewCmdPs Pss a command object for the "Ps" command
func NewCmdPs(p common.OptionsProvider) *cobra.Command {
	action := &PsAction{
		AgentOptions: agentcli.AgentOptions{
			CommonOptions: p(),
		},
	}

	cmd := &cobra.Command{
		Use:   "ps",
		Short: "Show Ziti process info",
		Long:  "Show information about currently running Ziti preocesses",
		Run: func(cmd *cobra.Command, args []string) {
			action.Cmd = cmd
			action.Args = args
			err := action.Run()
			cmdhelper.CheckErr(err)
		},
	}

	cmd.AddCommand(agentcli.NewGetCmd(p))
	cmd.AddCommand(agentcli.NewGoVersionCmd(p))
	cmd.AddCommand(agentcli.NewGcCmd(p))
	cmd.AddCommand(agentcli.NewSetGcCmd(p))
	cmd.AddCommand(agentcli.NewStackCmd(p))
	cmd.AddCommand(agentcli.NewMemstatsCmd(p))
	cmd.AddCommand(agentcli.NewStatsCmd(p))
	cmd.AddCommand(agentcli.NewPprofHeapCmd(p))
	cmd.AddCommand(agentcli.NewPprofCpuCmd(p))
	cmd.AddCommand(agentcli.NewTraceCmd(p))
	cmd.AddCommand(agentcli.NewSetLogLevelCmd(p))
	cmd.AddCommand(agentcli.NewSetChannelLogLevelCmd(p))
	cmd.AddCommand(agentcli.NewClearChannelLogLevelCmd(p))

	return cmd
}

// Run implements this command
func (self *PsAction) Run() error {
	fmt.Println("ps is running")
	ps := goprocess.FindAll()

	var maxPID, maxPPID, maxExec, maxVersion int
	for i, p := range ps {

		fmt.Println("p.Path is: " + p.Path)

		ps[i].BuildVersion = shortenVersion(p.BuildVersion)
		maxPID = max(maxPID, len(strconv.Itoa(p.PID)))
		maxPPID = max(maxPPID, len(strconv.Itoa(p.PPID)))
		maxExec = max(maxExec, len(p.Exec))
		maxVersion = max(maxVersion, len(ps[i].BuildVersion))

	}

	for _, p := range ps {
		buf := bytes.NewBuffer(nil)
		pid := strconv.Itoa(p.PID)
		fmt.Fprint(buf, pad(pid, maxPID))
		fmt.Fprint(buf, " ")

		ppid := strconv.Itoa(p.PPID)
		fmt.Fprint(buf, pad(ppid, maxPPID))
		fmt.Fprint(buf, " ")

		fmt.Fprint(buf, pad(p.Exec, maxExec))

		if p.Agent {
			fmt.Fprint(buf, "*")
		} else {
			fmt.Fprint(buf, " ")
		}
		fmt.Fprint(buf, " ")

		fmt.Fprint(buf, pad(p.BuildVersion, maxVersion))
		fmt.Fprint(buf, " ")

		fmt.Fprint(buf, p.Path)
		fmt.Fprintln(buf)
		buf.WriteTo(os.Stdout)
	}

	return nil
}

var develRe = regexp.MustCompile(`devel\s+\+\w+`)

func shortenVersion(v string) string {
	if !strings.HasPrefix(v, "devel") {
		return v
	}
	results := develRe.FindAllString(v, 1)
	if len(results) == 0 {
		return v
	}
	return results[0]
}

func pad(s string, total int) string {
	if len(s) >= total {
		return s
	}
	return s + strings.Repeat(" ", total-len(s))
}

func max(i, j int) int {
	if i > j {
		return i
	}
	return j
}
