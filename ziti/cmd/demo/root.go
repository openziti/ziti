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

package demo

import (
	"github.com/openziti/ziti/ziti/cmd/agentcli"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/spf13/cobra"
)

func NewDemoCmd(p common.OptionsProvider) *cobra.Command {
	demoCmd := &cobra.Command{
		Use:   "demo",
		Short: "Demos and examples for learning about Ziti",
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			helpers.CheckErr(err)
		},
	}

	setupCmd := &cobra.Command{
		Use:   "setup",
		Short: "Setup various demos/examples",
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			helpers.CheckErr(err)
		},
	}

	echoCmd := &cobra.Command{
		Use:   "echo",
		Short: "Setup various echo service demos/examples",
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			helpers.CheckErr(err)
		},
	}

	agentCmd := agentcli.NewAgentCmd(p)
	echoServerAgentCmd := &cobra.Command{
		Use:   "echo-server",
		Short: "Interact with an echo-server process using the IPC agent",
		Run: func(cmd *cobra.Command, args []string) {
			helpers.CheckErr(cmd.Help())
		},
	}

	agentCmd.AddCommand(echoServerAgentCmd)
	echoServerAgentCmd.AddCommand(NewAgentEchoServerUpdateTerminatorCmd(p))

	demoCmd.AddCommand(newEchoServerCmd())
	demoCmd.AddCommand(newZcatCmd())
	demoCmd.AddCommand(agentCmd)

	demoCmd.AddCommand(setupCmd)
	setupCmd.AddCommand(echoCmd)
	echoCmd.AddCommand(newClientCmd(p))
	echoCmd.AddCommand(newRouterTunnelerBothSidesCmd(p))
	echoCmd.AddCommand(newSingleSdkHostedCmd(p))
	echoCmd.AddCommand(newMultiSdkHostedCmd(p))
	echoCmd.AddCommand(newSingleRouterTunnelerHostedCmd(p))
	echoCmd.AddCommand(newMultiRouterTunnelerHostedCmd(p))
	echoCmd.AddCommand(newMultiTunnelerHostedCmd(p))
	echoCmd.AddCommand(newUpdateConfigAddressableCmd(p))
	echoCmd.AddCommand(newUpdateConfigHACmd(p))

	return demoCmd
}
