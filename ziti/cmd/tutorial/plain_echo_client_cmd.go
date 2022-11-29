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

package tutorial

import (
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/spf13/cobra"
	"strings"
)

// Options are common options for edge controller commands
type plainEchoClientOptions struct {
	common.CommonOptions
	port uint16
	host string
}

func (self *plainEchoClientOptions) run() error {
	echoClient := &plainEchoClient{
		host: self.host,
		port: self.port,
	}
	return echoClient.echo(strings.Join(self.Args, " "))
}

func newPlainEchoClientCmd(p common.OptionsProvider) *cobra.Command {
	options := &plainEchoClientOptions{
		CommonOptions: p(),
	}

	cmd := &cobra.Command{
		Use:   "plain-echo-client strings to echo",
		Short: "Runs a simple http echo client",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.run()
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().Uint16Var(&options.port, "port", 80, "Specify the port to dial on")
	cmd.Flags().StringVar(&options.host, "host", "localhost", "Specify the host to connect to")

	return cmd
}
