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
	"io"

	"github.com/spf13/cobra"

	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/templates"
)

// Update contains the command line options
type UpdateOptions struct {
	CommonOptions

	DisableImport bool
	OutDir        string
}

var (
	updateResources = `Valid resource types include:

	* config
	`

	updateLong = templates.LongDesc(`
		Updates an existing resource.

		` + updateResources + `
`)
)

// NewCmdUpdate creates a command object for the "update" command
func NewCmdUpdate(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &UpdateOptions{
		CommonOptions: CommonOptions{
			Out: out,
			Err: errOut,
		},
	}

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Updates an existing resource",
		Long:  updateLong,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}

	// cmd.AddCommand(NewCmdUpdateConfig(f, out, errOut))

	return cmd
}

// Run implements this command
func (o *UpdateOptions) Run() error {
	return o.Cmd.Help()
}
