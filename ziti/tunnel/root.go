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
	"github.com/openziti/sdk-golang/ziti/sdkinfo"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/openziti/ziti/ziti/util"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/agent"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/ziti/common/version"
	"github.com/openziti/ziti/tunnel"
	"github.com/openziti/ziti/tunnel/dns"
	"github.com/openziti/ziti/tunnel/entities"
	"github.com/openziti/ziti/tunnel/intercept"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const (
	svcPollRateFlag   = "svcPollRate"
	resolverCfgFlag   = "resolver"
	dnsSvcIpRangeFlag = "dnsSvcIpRange"
)

var hostSpecificCmds []func() *cobra.Command

func NewTunnelCmd(legacy bool) *cobra.Command {
	var root = &cobra.Command{
		Use:              "tunnel",
		Short:            "Ziti Tunnel",
		PersistentPreRun: rootPreRun,
		Hidden:           true,
	}

	root.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose mode")
	root.PersistentFlags().StringP("identity", "i", "", "Path to JSON file that contains an enrolled identity")
	root.PersistentFlags().String("identity-dir", "", "Path to directory file that contains one or more enrolled identities")
	root.PersistentFlags().Uint(svcPollRateFlag, 15, "Set poll rate for service updates (seconds). Polling in proxy mode is disabled unless this value is explicitly set")
	root.PersistentFlags().StringP(resolverCfgFlag, "r", "udp://127.0.0.1:53", "Resolver configuration")
	root.PersistentFlags().StringVar(&logFormatter, "log-formatter", "", "Specify log formatter [json|pfxlog|text]")
	root.PersistentFlags().StringP(dnsSvcIpRangeFlag, "d", "100.64.0.1/10", "cidr to use when assigning IPs to unresolvable intercept hostnames")
	root.PersistentFlags().BoolVar(&cliAgentEnabled, "cli-agent", true, "Enable/disable CLI Agent (enabled by default)")
	root.PersistentFlags().StringVar(&cliAgentAddr, "cli-agent-addr", "", "Specify where CLI Agent should list (ex: unix:/tmp/myfile.sock or tcp:127.0.0.1:10001)")
	root.PersistentFlags().StringVar(&cliAgentAlias, "cli-agent-alias", "", "Alias which can be used by ziti agent commands to find this instance")
	root.PersistentFlags().BoolVar(&ha, "ha", false, "Enable HA controller compatibility")

	root.AddCommand(NewHostCmd())
	root.AddCommand(NewProxyCmd())
	for _, cmdF := range hostSpecificCmds {
		cmd := cmdF()
		if cmd.Name() != "run" || legacy { // only include run in 'ziti tunnel' tree
			root.AddCommand(cmdF())
		}
	}

	versionCmd := common.NewVersionCmd()
	versionCmd.Hidden = true
	versionCmd.Deprecated = "use 'ziti version' instead of 'ziti router version'"
	root.AddCommand(versionCmd)

	return root
}

var interceptor intercept.Interceptor
var logFormatter string
var cliAgentEnabled bool
var cliAgentAddr string
var cliAgentAlias string
var ha bool

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
	util.LogReleaseVersionCheck()
}

func rootPostRun(cmd *cobra.Command, _ []string) {
	log := pfxlog.Logger()

	if cliAgentEnabled {
		// don't use the agent's shutdown handler. it calls os.Exit on SIGINT
		// which interferes with the servicePoller shutdown
		cleanup := false
		err := agent.Listen(agent.Options{
			Addr:            cliAgentAddr,
			ShutdownCleanup: &cleanup,
			AppAlias:        cliAgentAlias,
		})

		if err != nil {
			pfxlog.Logger().WithError(err).Error("unable to start CLI agent")
		}
	}

	sdkinfo.SetApplication("ziti-tunnel", version.GetVersion())

	resolverConfig := cmd.Flag(resolverCfgFlag).Value.String()
	resolver, err := dns.NewResolver(resolverConfig)
	if err != nil {
		log.WithError(err).Fatal("failed to start DNS resolver")
	}

	serviceListenerGroup := intercept.NewServiceListenerGroup(interceptor, resolver)

	dnsIpRange, _ := cmd.Flags().GetString(dnsSvcIpRangeFlag)
	if err := intercept.SetDnsInterceptIpRange(dnsIpRange); err != nil {
		log.Fatalf("invalid dns service IP range %s: %v", dnsIpRange, err)
	}

	if idDir := cmd.Flag("identity-dir").Value.String(); idDir != "" {
		files, err := os.ReadDir(idDir)
		if err != nil {
			log.Fatalf("failed to scan directory %s: %v", idDir, err)
		}

		for _, file := range files {
			if filepath.Ext(file.Name()) == ".json" {
				fn, err := filepath.Abs(filepath.Join(idDir, file.Name()))
				if err != nil {
					log.Fatalf("failed to listing file %s: %v", file.Name(), err)
				}
				go startIdentity(cmd, serviceListenerGroup, fn)
			}
		}
	} else {
		identityJson := cmd.Flag("identity").Value.String()
		startIdentity(cmd, serviceListenerGroup, identityJson)
	}

	serviceListenerGroup.WaitForShutdown()

	if cliAgentEnabled {
		agent.Close()
	}
}

func startIdentity(cmd *cobra.Command, serviceListenerGroup *intercept.ServiceListenerGroup, identityJson string) {
	log := pfxlog.Logger()

	log.Infof("loading identity: %v", identityJson)
	zitiCfg, err := ziti.NewConfigFromFile(identityJson)
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

	zitiCfg.MaxControlConnections = 1

	serviceListener := serviceListenerGroup.NewServiceListener()
	svcPollRate, _ := cmd.Flags().GetUint(svcPollRateFlag)
	options := &ziti.Options{
		RefreshInterval: time.Duration(svcPollRate) * time.Second,
		OnContextReady: func(ctx ziti.Context) {
			serviceListener.HandleProviderReady(tunnel.NewContextProvider(ctx))
		},
		OnServiceUpdate: serviceListener.HandleServicesChange,
		EdgeRouterUrlFilter: func(url string) bool {
			return strings.HasPrefix(url, "tls:")
		},
	}

	rootPrivateContext, err := ziti.NewContextWithOpts(zitiCfg, options)

	if err != nil {
		pfxlog.Logger().WithError(err).Fatal("could not create ziti sdk context")
	}

	if ha {
		rootPrivateContext.(*ziti.ContextImpl).CtrlClt.SetUseOidc(true)
	}

	for {
		if err = rootPrivateContext.Authenticate(); err != nil {
			log.WithError(err).Error("failed to authenticate")
			time.Sleep(30 * time.Second)
		} else {
			return
		}
	}
}
