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
	"encoding/binary"
	"github.com/openziti/foundation/agent"
	cmdutil "github.com/openziti/ziti/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/signal"
	"github.com/spf13/cobra"
	"io"
	"os"
	"strconv"
)

// PsSetgcOptions the options for the create spring command
type PsSetgcOptions struct {
	PsOptions
	CtrlListener string
}

// NewCmdPsSetgc creates a command object for the "create" command
func NewCmdPsSetgc(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &PsSetgcOptions{
		PsOptions: PsOptions{
			CommonOptions: CommonOptions{
				Factory: f,
				Out:     out,
				Err:     errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:  "setgc target gc-percentage",
		Args: cobra.MinimumNArgs(1),
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
func (o *PsSetgcOptions) Run() error {
	var addr string
	var err error
	var pctArg string
	if len(o.Args) == 1 {
		addr, err = agent.ParseGopsAddress(nil)
		pctArg = o.Args[0]
	} else {
		addr, err = agent.ParseGopsAddress(o.Args)
		pctArg = o.Args[1]
	}

	if err != nil {
		return err
	}

	perc, err := strconv.ParseInt(pctArg, 10, strconv.IntSize)
	if err != nil {
		return err
	}
	buf := make([]byte, binary.MaxVarintLen64)
	binary.PutVarint(buf, perc)

	return agent.MakeRequest(addr, signal.SetGCPercent, buf, os.Stdout)
}
