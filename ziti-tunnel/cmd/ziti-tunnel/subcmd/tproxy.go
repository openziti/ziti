// +build linux

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
	"github.com/netfoundry/ziti-edge/tunnel/intercept/tproxy"
	"github.com/spf13/cobra"
)

var runTProxyCmd = &cobra.Command{
	Use:     "tproxy",
	Short:   "Use the 'tproxy' interceptor",
	Long:    "The 'tproxy' interceptor captures packets by using the TPROXY iptables target.",
	RunE:    runTProxy,
	PostRun: rootPostRun,
}

func init() {
	root.AddCommand(runTProxyCmd)
}

func runTProxy(cmd *cobra.Command, args []string) error {
	var err error
	interceptor, err = tproxy.New()
	if err != nil {
		return fmt.Errorf("failed to initialize tproxy interceptor: %v", err)
	}
	return nil
}
