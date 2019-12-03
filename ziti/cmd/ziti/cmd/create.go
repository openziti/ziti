/*
	Copyright 2019 Netfoundry, Inc.

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

	cmdutil "github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/helpers"
	"github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/templates"
)

// CreateOptions contains the command line options
type CreateOptions struct {
	CommonOptions

	DisableImport bool
	OutDir        string
}

var (
	createLong = templates.LongDesc(`
		Creates a new Ziti resource.

	`)
)

// NewCmdCreate creates a command object for the "create" command
func NewCmdCreate(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &CreateOptions{
		CommonOptions: CommonOptions{
			Factory: f,
			Out:     out,
			Err:     errOut,
		},
	}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new resource",
		Long:  createLong,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}

	cmd.AddCommand(NewCmdCreateConfig(f, out, errOut))
	// cmd.AddCommand(NewCmdCreateStateStore(f, out, errOut))
	cmd.AddCommand(NewCmdCreateEnvironment(f, out, errOut))

	cmd.AddCommand(NewCmdPKICreateCA(f, out, errOut))
	cmd.AddCommand(NewCmdPKICreateIntermediate(f, out, errOut))
	cmd.AddCommand(NewCmdPKICreateServer(f, out, errOut))
	cmd.AddCommand(NewCmdPKICreateClient(f, out, errOut))
	cmd.AddCommand(NewCmdPKICreateCSR(f, out, errOut))

	return cmd
}

// Run implements this command
func (o *CreateOptions) Run() error {
	return o.Cmd.Help()
}
