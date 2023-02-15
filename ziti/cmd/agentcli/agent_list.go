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
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/openziti/agent"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/spf13/cobra"
	"os"
)

type AgentListAction struct {
	AgentOptions
}

// NewListCmd Pss a command object for the "list" command
func NewListCmd(p common.OptionsProvider) *cobra.Command {
	action := &AgentListAction{
		AgentOptions: AgentOptions{
			CommonOptions: p(),
		},
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List ziti processes",
		Long:  "Show information about currently running Ziti processes",
		Run: func(cmd *cobra.Command, args []string) {
			action.Cmd = cmd
			action.Args = args
			err := action.Run()
			cmdhelper.CheckErr(err)
		},
	}

	return cmd
}

func (self *AgentListAction) Run() error {
	processes, err := agent.GetGopsProcesses()
	if err != nil {
		return err
	}

	t := table.NewWriter()
	t.SetStyle(table.StyleRounded)
	t.AppendHeader(table.Row{"PID", "Executable", "App ID", "Unix Socket", "App Type", "App Version", "App Alias"})

	for _, p := range processes {
		t.AppendRow(table.Row{p.Pid, p.Executable, p.AppId, p.UnixSocket, p.AppType, p.AppVersion, p.AppAlias})
	}

	if _, err = fmt.Fprintln(os.Stdout, t.Render()); err != nil {
		panic(err)
	}
	return nil
}
