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

// InstallOptions are the flags for delete commands
type InstallOptions struct {
	CommonOptions
}

var (
	install_long = templates.LongDesc(`
		Install the Ziti platform binaries.
`)

	install_example = templates.Examples(`
		# install the Ziti router
		ziti install ziti-router
	`)
)

// NewCmdInstall creates the command
func NewCmdInstall(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &InstallOptions{
		CommonOptions{
			Out: out,
			Err: errOut,
		},
	}

	cmd := &cobra.Command{
		Use:     "install [flags]",
		Short:   "Installs a Ziti component/app",
		Long:    install_long,
		Example: install_example,
		Aliases: []string{"install"},
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{"up"},
	}

	cmd.AddCommand(NewCmdInstallTerraformProviderEdgeController(out, errOut))

	options.AddCommonFlags(cmd)

	return cmd
}

// Run implements this command
func (o *InstallOptions) Run() error {
	return o.Cmd.Help()
}
