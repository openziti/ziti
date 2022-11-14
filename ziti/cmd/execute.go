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
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/templates"
	"io"

	"github.com/spf13/cobra"
)

// ExecuteOptions contains the command line options
type ExecuteOptions struct {
	CommonOptions

	Flags ExecuteFlags

	DisableImport bool
	OutDir        string
}

type ExecuteFlags struct {
	Identity string
}

var (
	executeLong = templates.LongDesc(`
		Executes a component.
	`)
)

// NewCmdExecute Executes a command object for the "Execute" command
func NewCmdExecute(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &ExecuteOptions{
		CommonOptions: CommonOptions{
			Out: out,
			Err: errOut,
		},
	}

	cmd := &cobra.Command{
		Use:     "run",
		Short:   "Execute a Ziti component/app",
		Long:    executeLong,
		Aliases: []string{},
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}

	cmd.AddCommand(NewCmdExecuteController(out, errOut))

	options.addExecuteFlags(cmd)
	return cmd
}

func (options *ExecuteOptions) addExecuteFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&options.Flags.Identity, "identity", "", "", "Which identity to use.")
}

// Run implements this command
func (o *ExecuteOptions) Run() error {
	return o.Cmd.Help()
}
