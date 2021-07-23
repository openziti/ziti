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
	"github.com/michaelquigley/pfxlog"
	cmdutil "github.com/openziti/ziti/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/spf13/cobra"
	"io"
)

// PsOptions contains the command line options
type LogFormatOptions struct {
	CommonOptions

	absoluteTime bool
	trimPrefix   string
}

// NewCmdLogFormat a command object for the "log-format" command
func NewCmdLogFormat(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &LogFormatOptions{
		CommonOptions: CommonOptions{
			Factory: f,
			Out:     out,
			Err:     errOut,
		},
	}

	cmd := &cobra.Command{
		Use:     "log-format",
		Short:   "Transform pfxlog output into a human readable format",
		Aliases: []string{"lf"},
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}

	cmd.Flags().BoolVarP(&options.absoluteTime, "absolute", "a", false, "Use absolute time for timestamps")
	cmd.Flags().StringVarP(&options.trimPrefix, "trim", "t", "", "Trim package prefix (ex: github.com/michaelquigley/)")

	return cmd
}

// Run implements this command
func (o *LogFormatOptions) Run() error {
	options := pfxlog.DefaultOptions().SetTrimPrefix(o.trimPrefix)
	if o.absoluteTime {
		options.SetAbsoluteTime()
	}
	pfxlog.Filter(options)
	return nil
}
