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
	"github.com/openziti/edge/tunnel/dns"
	"github.com/openziti/edge/tunnel/entities"
	"github.com/openziti/edge/tunnel/intercept"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/config"
	"github.com/openziti/ziti/common/enrollment"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"time"
)

const (
	svcPollRateFlag = "svcPollRate"
	resolverCfgFlag = "resolver"
)

func init() {
	root.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose mode")
	root.PersistentFlags().StringP("identity", "i", "", "Path to JSON file that contains an enrolled identity")
	root.PersistentFlags().Uint(svcPollRateFlag, 15, "Set poll rate for service updates (seconds)")
	root.PersistentFlags().StringP(resolverCfgFlag, "r", "udp://127.0.0.1:53", "Resolver configuration")
	root.PersistentFlags().StringVar(&logFormatter, "log-formatter", "", "Specify log formatter [json|pfxlog|text]")

	root.AddCommand(enrollment.NewEnrollCommand())
}

var root = &cobra.Command{
	Use:              filepath.Base(os.Args[0]),
	Short:            "Ziti Tunnel",
	PersistentPreRun: rootPreRun,
}

var interceptor intercept.Interceptor
var resolver dns.Resolver
var logFormatter string

func Execute() {
	if err := root.Execute(); err != nil {
		pfxlog.Logger().Errorf("error: %s", err)
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
		logrus.SetFormatter(pfxlog.NewFormatterStartingToday())
	case "json":
		logrus.SetFormatter(&logrus.JSONFormatter{})
	case "text":
		logrus.SetFormatter(&logrus.TextFormatter{})
	default:
		// let logrus do its own thing
	}
}

func rootPostRun(cmd *cobra.Command, _ []string) {
	log := pfxlog.Logger()

	identityJson := cmd.Flag("identity").Value.String()
	zitiCfg, err := config.NewFromFile(identityJson)
	if err != nil {
		log.Fatalf("failed to load ziti configuration from %s: %v", identityJson, err)
	}
	zitiCfg.ConfigTypes = []string{
		entities.ClientConfigV1,
		entities.ServerConfigV1,
	}
	rootPrivateContext := ziti.NewContextWithConfig(zitiCfg)
	if err != nil {
		log.Fatalf("failed to initialize Ziti SDK: %v", err)
	}

	svcPollRate, _ := cmd.Flags().GetUint(svcPollRateFlag)
	resolverConfig := cmd.Flag("resolver").Value.String()
	resolver = dns.NewResolver(resolverConfig)

	intercept.ServicePoller(rootPrivateContext, interceptor, resolver, time.Duration(svcPollRate)*time.Second)
}
