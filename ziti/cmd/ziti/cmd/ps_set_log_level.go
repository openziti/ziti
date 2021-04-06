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
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"io"
	"os"
	"strings"
)

// PsSetgcOptions the options for the create spring command
type PsSetLogLevelOptions struct {
	PsOptions
	CtrlListener string
}

// NewCmdPsLogLevel creates a command object for the "create" command
func NewCmdPsSetLogLevel(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &PsSetLogLevelOptions{
		PsOptions: PsOptions{
			CommonOptions: CommonOptions{
				Factory: f,
				Out:     out,
				Err:     errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:  "set-log-level target log-level (panic, fatal, error, warn, info, debug, trace)",
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
func (o *PsSetLogLevelOptions) Run() error {
	var addr string
	var err error
	var levelArg string
	if len(o.Args) == 1 {
		addr, err = agent.ParseGopsAddress(nil)
		levelArg = o.Args[0]
	} else {
		addr, err = agent.ParseGopsAddress(o.Args)
		levelArg = o.Args[1]
	}

	if err != nil {
		return err
	}

	var level logrus.Level
	var found bool
	for _, l := range logrus.AllLevels {
		if strings.EqualFold(l.String(), levelArg) {
			level = l
			found = true
		}
	}

	if !found {
		return errors.Errorf("invalid log level %v", levelArg)
	}

	buf := []byte{byte(level)}
	return agent.MakeRequest(addr, agent.SetLogLevel, buf, os.Stdout)
}
