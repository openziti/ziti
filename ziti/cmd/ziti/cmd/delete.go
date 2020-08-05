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
	"io"

	"github.com/spf13/cobra"

	cmdutil "github.com/openziti/ziti/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/templates"
)

// DeleteOptions are the flags for delete commands
type DeleteOptions struct {
	CommonOptions
}

var (
	deleteLong = templates.LongDesc(`
		Deletes a resource.
	`)

	deleteExample = templates.Examples(`
		# Delete the controller config
		ziti delete config controller
	`)
)

// NewCmdDelete creates a command object for the generic "delete" action
func NewCmdDelete(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &DeleteOptions{
		CommonOptions{
			Factory: f,
			Out:     out,
			Err:     errOut,
		},
	}

	cmd := &cobra.Command{
		Use:     "delete TYPE [flags]",
		Short:   "Deletes one or many resources",
		Long:    deleteLong,
		Example: deleteExample,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{"remove", "rm", "del"},
	}

	cmd.AddCommand(NewCmdDeleteStateStore(f, out, errOut))

	return cmd
}

// Run implements this command
func (o *DeleteOptions) Run() error {
	return o.Cmd.Help()
}
