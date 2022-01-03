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
	"fmt"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	optionVerbose      = "verbose"
	defaultVerbose     = false
	verboseDescription = "Enable verbose logging. Logging will be sent to stdout if the config output is sent to a file. If output is sent to stdout, logging will be sent to stderr"
	optionOutput       = "output"
	defaultOutput      = "stdout"
	outputDescription  = "designated output destination for config, use \"stdout\" or a filepath."
)

// CreateConfigOptions the options for the create spring command
type CreateConfigOptions struct {
	common.CommonOptions

	Output                   string
	DatabaseFile             string
	ZitiPKI                  string
	Verbose                  bool
	ZitiCtrlIntermediateName string
	ZitiCtrlHostname         string
	ZitiFabCtrlPort          string
}

// NewCmdCreateConfig creates a command object for the "config" command
func NewCmdCreateConfig(p common.OptionsProvider) *cobra.Command {
	options := &CreateConfigOptions{
		CommonOptions: p(),
	}

	cmd := &cobra.Command{
		Use:     "config",
		Short:   "Creates a config file for specified Ziti component",
		Aliases: []string{"cfg"},
		Run: func(cmd *cobra.Command, args []string) {
			cmdhelper.CheckErr(cmd.Help())
		},
	}

	options.addCreateFlags(cmd)

	fmt.Printf("2#### output is: `%s`\n", options.Output)

	cmd.AddCommand(NewCmdCreateConfigController(options.Factory, options.Out, options.Err))
	//cmd.AddCommand(NewCmdCreateEnvironment(f, out, errOut))
	cmd.AddCommand(NewCmdCreateConfigRouter(options.Factory, options.Out, options.Err))

	return cmd
}

// Add flags that are global to all "create config" commands
func (options *CreateConfigOptions) addCreateFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().BoolP(optionVerbose, "v", defaultVerbose, verboseDescription)
	cmd.PersistentFlags().StringVar(&options.Output, "q", defaultOutput, outputDescription)
	viper.BindPFlag(optionVerbose, cmd.PersistentFlags().Lookup(optionVerbose))
	viper.BindPFlag(optionOutput, cmd.PersistentFlags().Lookup(optionOutput))
}
