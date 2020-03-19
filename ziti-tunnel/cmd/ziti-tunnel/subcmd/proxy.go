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
	"github.com/netfoundry/ziti-edge/tunnel/intercept"
	"github.com/netfoundry/ziti-edge/tunnel/intercept/proxy"
	"github.com/spf13/cobra"
	"math"
	"net"
	"strconv"
	"strings"
)

var runProxyCmd = &cobra.Command{
	Use:     "proxy <service-name:port> [sevice-name:port]",
	Short:   "Run in 'proxy' mode",
	Long:    "The 'proxy' intercept mode creates a network listener for each service that is intercepted.",
	Args:    cobra.MinimumNArgs(1),
	RunE:    runProxy,
	PostRun: rootPostRun,
}

func init() {
	root.AddCommand(runProxyCmd)
}

func runProxy(_ *cobra.Command, args []string) error {
	services := make(map[string]*proxy.Service, len(args))

	for _, arg := range args {
		parts := strings.Split(arg, ":")
		if len(parts) < 2 || len(parts) > 3 {
			return fmt.Errorf("invalid argument '%s'", arg)
		}

		port, err := strconv.Atoi(parts[1])
		if err != nil {
			return fmt.Errorf("invalid port specified in '%s'", arg)
		}

		service := &proxy.Service{
			Name:     parts[0],
			Port:     port,
			Protocol: intercept.TCP,
		}

		if len(parts) == 3 {
			protocol := parts[2]
			if protocol == "udp" {
				service.Protocol = intercept.UDP
			} else if protocol != "tcp" {
				return fmt.Errorf("invalid protocol specified in '%s', must be tcp or udp", arg)
			}
		}
		services[parts[0]] = service
	}

	// Fiddle with the poll rate and resolver settings if the user didn't wan't anything special.
	if !root.Flag(svcPollRateFlag).Changed {
		_ = root.PersistentFlags().Set(svcPollRateFlag, strconv.FormatUint(math.MaxUint32, 10))
	}
	if !root.Flag(resolverCfgFlag).Changed {
		_ = root.PersistentFlags().Set(resolverCfgFlag, "")
	}
	var err error
	interceptor, err = proxy.New(net.IPv4zero, services)
	if err != nil {
		return fmt.Errorf("failed to initialize proxy interceptor: %v", err)
	}
	return nil
}
