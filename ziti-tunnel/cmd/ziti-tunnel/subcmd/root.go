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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/tunnel"
	"github.com/openziti/edge/tunnel/dns"
	"github.com/openziti/edge/tunnel/entities"
	"github.com/openziti/edge/tunnel/intercept"
	"github.com/openziti/foundation/agent"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/config"
	"github.com/openziti/ziti/common/enrollment"
	"github.com/openziti/ziti/common/version"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"time"
)

const (
	svcPollRateFlag   = "svcPollRate"
	resolverCfgFlag   = "resolver"
	dnsSvcIpRangeFlag = "dnsSvcIpRange"
)

func init() {
	root.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose mode")
	root.PersistentFlags().StringP("identity", "i", "", "Path to JSON file that contains an enrolled identity")
	root.PersistentFlags().Uint(svcPollRateFlag, 15, "Set poll rate for service updates (seconds). Polling in proxy mode is disabled unless this value is explicitly set")
	root.PersistentFlags().StringP(resolverCfgFlag, "r", "udp://127.0.0.1:53", "Resolver configuration")
	root.PersistentFlags().StringVar(&logFormatter, "log-formatter", "", "Specify log formatter [json|pfxlog|text]")
	root.PersistentFlags().StringP(dnsSvcIpRangeFlag, "d", "100.64.0.1/10", "cidr to use when assigning IPs to unresolvable intercept hostnames")
	root.PersistentFlags().BoolVar(&cliAgentEnabled, "cli-agent", true, "Enable/disable CLI Agent (enabled by default)")
	root.PersistentFlags().StringVar(&cliAgentAddr, "cli-agent-addr", "", "Specify where CLI Agent should list (ex: unix:/tmp/myfile.sock or tcp:127.0.0.1:10001)")

	root.AddCommand(enrollment.NewEnrollCommand())
}

var root = &cobra.Command{
	Use:              filepath.Base(os.Args[0]),
	Short:            "Ziti Tunnel",
	PersistentPreRun: rootPreRun,
}

var interceptor intercept.Interceptor
var logFormatter string
var cliAgentEnabled bool
var cliAgentAddr string

func Execute() {
	if err := root.Execute(); err != nil {
		pfxlog.Logger().Errorf("error: %s", err)
		os.Exit(1)
	}
}

func rootPreRun(cmd *cobra.Command, _ []string) {
	verbose, err := cmd.Flags().GetBool("verbose")
	if err != nil {
		println("err")
	}
	if verbose {
		logrus.SetLevel(logrus.DebugLevel)
	}

	switch logFormatter {
	case "pfxlog":
		logrus.SetFormatter(pfxlog.NewFormatter(pfxlog.DefaultOptions().StartingToday()))
	case "json":
		logrus.SetFormatter(&logrus.JSONFormatter{TimestampFormat: "2006-01-02T15:04:05.000Z"})
	case "text":
		logrus.SetFormatter(&logrus.TextFormatter{})
	default:
		// let logrus do its own thing
	}
}

func rootPostRun(cmd *cobra.Command, _ []string) {
	log := pfxlog.Logger()

	if cliAgentEnabled {
		// don't use the agent's shutdown handler. it calls os.Exit on SIGINT
		// which interferes with the servicePoller shutdown
		cleanup := false
		if err := agent.Listen(agent.Options{Addr: cliAgentAddr, ShutdownCleanup: &cleanup}); err != nil {
			pfxlog.Logger().WithError(err).Error("unable to start CLI agent")
		}
	}

	ziti.SetApplication("ziti-tunnel", version.GetVersion())

	identityJson := cmd.Flag("identity").Value.String()
	zitiCfg, err := config.NewFromFile(identityJson)
	if err != nil {
		log.Fatalf("failed to load ziti configuration from %s: %v", identityJson, err)
	}
	zitiCfg.ConfigTypes = []string{
		entities.ClientConfigV1,
		entities.ServerConfigV1,
		entities.InterceptV1,
		entities.HostConfigV1,
		entities.HostConfigV2,
	}

	resolverConfig := cmd.Flag("resolver").Value.String()
	resolver := dns.NewResolver(resolverConfig)

	serviceListener := intercept.NewServiceListener(interceptor, resolver)

	svcPollRate, _ := cmd.Flags().GetUint(svcPollRateFlag)
	options := &ziti.Options{
		RefreshInterval: time.Duration(svcPollRate) * time.Second,
		OnContextReady: func(ctx ziti.Context) {
			serviceListener.HandleProviderReady(tunnel.NewContextProvider(ctx))
		},
		OnServiceUpdate: serviceListener.HandleServicesChange,
	}

	rootPrivateContext := ziti.NewContextWithOpts(zitiCfg, options)

	dnsIpRange, _ := cmd.Flags().GetString(dnsSvcIpRangeFlag)
	if err = intercept.SetDnsInterceptIpRange(dnsIpRange); err != nil {
		log.Fatalf("invalid dns service IP range %s: %v", dnsIpRange, err)
	}

	interceptor.Start(tunnel.NewContextProvider(rootPrivateContext))

	if err = rootPrivateContext.Authenticate(); err != nil {
		log.WithError(err).Fatal("failed to authenticate")
	}
	serviceListener.WaitForShutdown()
	if cliAgentEnabled {
		agent.Close()
	}
}
