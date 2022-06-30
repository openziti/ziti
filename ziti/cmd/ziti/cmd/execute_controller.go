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
	"path/filepath"

	"github.com/spf13/cobra"

	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/templates"
	c "github.com/openziti/ziti/ziti/cmd/ziti/constants"
	"github.com/openziti/ziti/ziti/cmd/ziti/util"
	"github.com/spf13/viper"
)

var (
	executeControllerLong = templates.LongDesc(`
		Executes the ziti-controller component
	`)

	executeControllerExample = templates.Examples(`
		# Execute the ziti-controller component 
		ziti execute ziti-controller

	`)
)

// ExecuteControllerOptions the options for the create spring command
type ExecuteControllerOptions struct {
	ExecuteOptions

	CtrlListener string
}

// NewCmdExecuteController creates a command object for the "create" command
func NewCmdExecuteController(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &ExecuteControllerOptions{
		ExecuteOptions: ExecuteOptions{
			CommonOptions: CommonOptions{
				Out: out,
				Err: errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:     "controller",
		Short:   "'Execute the ziti-controller component'",
		Aliases: []string{"c", "ctrl"},
		Long:    executeControllerLong,
		Example: executeControllerExample,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}

	cmd.PersistentFlags().BoolVarP(&cliAgentEnabled, "cliagent", "a", false, "Enable CLI Agent (use in dev only)")

	options.AddCommonFlags(cmd)

	return cmd
}

// Run implements the command
func (o *ExecuteControllerOptions) Run() error {

	viper := viper.New()
	viper.SetConfigType("json")
	viper.SetConfigName(c.CONFIGFILENAME)
	zitiConfigDir, err := util.ZitiAppConfigDir(c.ZITI)
	if err != nil {
		return err
	}
	viper.AddConfigPath(zitiConfigDir)
	err = viper.ReadInConfig()
	if err != nil {
		return err
	}

	zitiControllerConfigDir, err := util.ZitiAppConfigDir(c.ZITI_CONTROLLER)
	if err != nil {
		return err
	}

	zitiControllerConfigFileName := filepath.Join(zitiControllerConfigDir, c.CONFIGFILENAME) + ".yml"

	err = o.startCommandFromDir(viper.GetString("bin"), c.ZITI_CONTROLLER, "run", zitiControllerConfigFileName)
	if err != nil {
		return err
	}

	return nil
}
