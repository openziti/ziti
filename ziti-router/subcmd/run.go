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
	"github.com/openziti/edge/router/xgress_edge"
	"github.com/openziti/edge/router/xgress_edge_transport"
	"github.com/openziti/fabric/router"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/foundation/agent"
	"github.com/openziti/ziti/common/version"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"io"
)

func init() {
	root.AddCommand(runCmd)
}

var runCmd = &cobra.Command{
	Use:   "run <config>",
	Short: "Run router configuration",
	Args:  cobra.ExactArgs(1),
	Run:   run,
}

func run(cmd *cobra.Command, args []string) {
	logrus.WithField("version", version.GetVersion()).
		WithField("go-version", version.GetGoVersion()).
		WithField("os", version.GetOS()).
		WithField("arch", version.GetArchitecture()).
		WithField("build-date", version.GetBuildDate()).
		WithField("revision", version.GetRevision()).
		Info("starting ziti-router")

	if config, err := router.LoadConfig(args[0]); err == nil {
		config.SetFlags(getFlags(cmd))

		r := router.Create(config, version.GetCmdBuildInfo())

		if cliAgentEnabled {
			options := agent.Options{Addr: cliAgentAddr}
			if debugOpsEnabled {
				options.CustomOps = map[byte]func(conn io.ReadWriter) error{
					agent.CustomOp: r.HandleDebug,
				}
			}
			if err := agent.Listen(options); err != nil {
				pfxlog.Logger().WithError(err).Error("unable to start CLI agent")
			}
		}

		config.SetFlags(getFlags(cmd))

		xgressEdgeFactory := xgress_edge.NewFactory(config, version.GetCmdBuildInfo())
		xgress.GlobalRegistry().Register("edge", xgressEdgeFactory)
		if err := r.RegisterXctrl(xgressEdgeFactory); err != nil {
			logrus.Panicf("error registering edge in framework (%v)", err)
		}

		xgressEdgeTransportFactory := xgress_edge_transport.NewFactory(config.Id, r)
		xgress.GlobalRegistry().Register(xgress_edge_transport.BindingName, xgressEdgeTransportFactory)

		if err := r.Run(); err != nil {
			logrus.WithError(err).Fatal("error starting")
		}

	} else {
		logrus.WithError(err).Fatal("error loading configuration")
	}
}

func getFlags(cmd *cobra.Command) map[string]*pflag.Flag {
	ret := map[string]*pflag.Flag{}
	cmd.Flags().Visit(func(f *pflag.Flag) {
		ret[f.Name] = f
	})
	return ret
}
