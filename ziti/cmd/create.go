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
	"github.com/spf13/cobra"
	"io"
)

var (
	createLong = templates.LongDesc(`
		Creates a new Ziti resource.

	`)
)

// NewCmdCreate creates a command object for the "create" command
func NewCmdCreate(out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new resource",
		Long:  createLong,
		Run: func(cmd *cobra.Command, args []string) {
			cmdhelper.CheckErr(cmd.Help())
		},
	}

	cmd.AddCommand(NewCmdCreateConfig())

	cmd.AddCommand(NewCmdPKICreateCA(out, errOut))
	cmd.AddCommand(NewCmdPKICreateIntermediate(out, errOut))
	cmd.AddCommand(NewCmdPKICreateServer(out, errOut))
	cmd.AddCommand(NewCmdPKICreateClient(out, errOut))
	cmd.AddCommand(NewCmdPKICreateCSR(out, errOut))

	return cmd
}
