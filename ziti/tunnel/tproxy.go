//go:build linux
// +build linux

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

package tunnel

import (
	"fmt"
	"github.com/openziti/edge/tunnel/intercept/tproxy"
	"github.com/spf13/cobra"
)

func init() {
	hostSpecificCmds = append(hostSpecificCmds, NewTProxyCmd())
}

func NewTProxyCmd() *cobra.Command {
	var runTProxyCmd = &cobra.Command{
		Use:     "tproxy",
		Short:   "Use the 'tproxy' interceptor",
		Long:    "The 'tproxy' interceptor captures packets by using the TPROXY iptables target.",
		RunE:    runTProxy,
		PostRun: rootPostRun,
	}
	runTProxyCmd.PersistentFlags().String("lanIf", "", "if specified, INPUT rules for intercepted service addresses are assigned to this interface ")
	return runTProxyCmd
}

func runTProxy(cmd *cobra.Command, args []string) error {
	var err error
	lanIf, err := cmd.Flags().GetString("lanIf")
	interceptor, err = tproxy.New(lanIf)
	if err != nil {
		return fmt.Errorf("failed to initialize tproxy interceptor: %v", err)
	}
	return nil
}
