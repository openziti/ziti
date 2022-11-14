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
	"github.com/openziti/ziti/ziti/internal/log"
	"github.com/openziti/ziti/ziti/util"
	"io"

	"github.com/spf13/cobra"
)

// CreateEnvironmentOptions the options for the create spring command
type CreateEnvironmentOptions struct {
	CommonOptions

	EnvName string
}

// NewCmdCreateEnvironment creates a command object for the "create" command
func NewCmdCreateEnvironment(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &CreateEnvironmentOptions{
		CommonOptions: CommonOptions{
			Out: out,
			Err: errOut,
		},
	}

	cmd := &cobra.Command{
		Use:     "environment",
		Short:   "Creates a 'deployment environment' directory, and seed files",
		Aliases: []string{"env"},
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}

	options.addFlags(cmd, "", "")
	return cmd
}

// Add flags that are global to all "create config" commands
func (o *CreateEnvironmentOptions) addFlags(cmd *cobra.Command, defaultNamespace string, defaultOptionRelease string) {
	cmd.Flags().StringVarP(&o.EnvName, "name", "", "", "Name of new 'environment'")
	cmd.MarkFlagRequired("name")
}

// Run implements this command
func (o *CreateEnvironmentOptions) Run() error {
	deps := make([]string, 0)
	deps = append(deps, "ansible")
	err := o.installMissingDependencies(deps)
	if err != nil {
		return err
	}

	envDir, err := util.NewEnvironmentDir(o.EnvName)
	if err != nil {
		return err
	}

	log.Infoln(envDir + " successfully created")

	return nil
}
