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
	"github.com/openziti/foundation/agent"
	cmdutil "github.com/openziti/ziti/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/spf13/cobra"
	"io"
	"os"
)

// PsPprofCpuOptions the options for the create spring command
type PsPprofCpuOptions struct {
	PsOptions
	CtrlListener string
}

// NewCmdPsPprofCpu creates a command object for the "create" command
func NewCmdPsPprofCpu(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &PsPprofCpuOptions{
		PsOptions: PsOptions{
			CommonOptions: CommonOptions{
				Factory: f,
				Out:     out,
				Err:     errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:  "pprof-cpu",
		Args: cobra.MaximumNArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}

	options.addCommonFlags(cmd)

	return cmd
}

// Run implements the command
func (o *PsPprofCpuOptions) Run() error {
	addr, err := agent.ParseGopsAddress(o.Args)
	if err != nil {
		return err
	}

	var out io.WriteCloser = os.Stdout
	if len(o.Args) > 1 {
		out, err = os.Create(o.Args[1])
		if err != nil {
			return err
		}
		defer out.Close()
	}

	return agent.MakeRequest(addr, agent.CPUProfile, nil, out)
}
