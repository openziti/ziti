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

package subcmd

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/server"
	"github.com/openziti/fabric/controller"
	"github.com/openziti/foundation/agent"
	"github.com/openziti/ziti/common/version"
	"github.com/spf13/cobra"
)

func init() {
	root.AddCommand(runCmd)
}

var runCmd = &cobra.Command{
	Use:   "run <config>",
	Short: "Run controller configuration",
	Args:  cobra.ExactArgs(1),
	Run:   run,
}

func run(cmd *cobra.Command, args []string) {
	if config, err := controller.LoadConfig(args[0]); err == nil {
		if cliAgentEnabled {
			if err := agent.Listen(agent.Options{Addr: cliAgentAddr}); err != nil {
				pfxlog.Logger().WithError(err).Error("unable to start CLI agent")
			}
		}

		var c *controller.Controller
		if c, err = controller.NewController(config, version.GetCmdBuildInfo()); err != nil {
			fmt.Printf("unable to create fabric controller %+v\n", err)
			panic(err)
		}

		ec, err := server.NewController(config)

		if err != nil {
			panic(err)
		}

		ec.SetHostController(c)
		ec.Initialize()

		go ec.RunAndWait()

		if err = c.Run(); err != nil {
			panic(err)
		}

	} else {
		panic(err)
	}
}
