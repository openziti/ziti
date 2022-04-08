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

package demo

import (
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/common"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/spf13/cobra"
)

func NewDemoCmd(p common.OptionsProvider) *cobra.Command {
	cmd := &cobra.Command{
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

	cmd.AddCommand(newEchoServerCmd())
	cmd.AddCommand(newZcatCmd())

	cmd.AddCommand(setupCmd)
	setupCmd.AddCommand(echoCmd)
	echoCmd.AddCommand(newClientCmd(p))
	echoCmd.AddCommand(newSingleSdkHostedCmd(p))
	echoCmd.AddCommand(newMultiSdkHostedCmd(p))
	echoCmd.AddCommand(newSingleRouterTunnelerHostedCmd(p))
	echoCmd.AddCommand(newMultiRouterTunnelerHostedCmd(p))
	echoCmd.AddCommand(newUpdateConfigAddressableCmd(p))
	echoCmd.AddCommand(newUpdateConfigHACmd(p))

	return cmd
}
