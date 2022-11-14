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
	"github.com/openziti/edge/tunnel/intercept/proxy"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"math"
	"net"
	"strconv"
)

func NewProxyCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "proxy <service-name:port> [sevice-name:port]",
		Short:   "Run in 'proxy' mode",
		Long:    "The 'proxy' intercept mode creates a network listener for each service that is intercepted.",
		Args:    cobra.MinimumNArgs(1),
		RunE:    runProxy,
		PostRun: rootPostRun,
	}
}

func runProxy(cmd *cobra.Command, args []string) error {
	root := cmd.Root()
	// Fiddle with the poll rate and resolver settings if the user didn't wan't anything special.
	if !root.Flag(svcPollRateFlag).Changed {
		_ = root.PersistentFlags().Set(svcPollRateFlag, strconv.FormatUint(math.MaxUint32, 10))
	}
	if !root.Flag(resolverCfgFlag).Changed {
		_ = root.PersistentFlags().Set(resolverCfgFlag, "")
	}
	var err error
	if interceptor, err = proxy.New(net.IPv4zero, args); err != nil {
		return errors.Wrap(err, "failed to initialize proxy interceptor")
	}
	return nil
}
