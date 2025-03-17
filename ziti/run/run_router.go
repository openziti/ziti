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

package run

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/agent"
	"github.com/openziti/foundation/v2/debugz"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/common/version"
	"github.com/openziti/ziti/router"
	"github.com/openziti/ziti/router/debugops"
	"github.com/openziti/ziti/router/xgress"
	"github.com/openziti/ziti/router/xgress_edge"
	"github.com/openziti/ziti/router/xgress_edge_transport"
	"github.com/openziti/ziti/router/xgress_edge_tunnel"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"net"
	"os"
	"os/signal"
	"syscall"
)

func NewRunRouterCmd() *cobra.Command {
	action := RouterAction{}
	var cmd = &cobra.Command{
		Use:    "router <config>",
		Short:  "Run router configuration",
		Args:   cobra.ExactArgs(1),
		Run:    action.run,
		PreRun: action.PreRun,
	}

	action.BindFlags(cmd)

	//flags are added to an internal map and read later on, see getFlags()
	cmd.Flags().BoolVar(&action.EnableDebugOps, "debug-ops", false, "Enable/disable debug agent operations (disabled by default)")
	cmd.Flags().BoolVarP(&action.ForceCertificateExtension, "extend", "e", false, "force the router on startup to extend enrollment certificates")
	return cmd
}

type RouterAction struct {
	Options
	EnableDebugOps            bool
	ForceCertificateExtension bool
}

func (self *RouterAction) run(cmd *cobra.Command, args []string) {
	startLogger := logrus.WithField("version", version.GetVersion()).
		WithField("go-version", version.GetGoVersion()).
		WithField("os", version.GetOS()).
		WithField("arch", version.GetArchitecture()).
		WithField("build-date", version.GetBuildDate()).
		WithField("revision", version.GetRevision()).
		WithField("configFile", args[0])

	config, err := router.LoadConfig(args[0])
	if err != nil {
		startLogger.WithError(err).Error("error loading ziti router config")
		panic(err)
	}

	config.Edge.ForceExtendEnrollment = self.ForceCertificateExtension

	startLogger = startLogger.WithField("routerId", config.Id.Token)
	startLogger.Info("starting ziti router")

	r := router.Create(config, version.GetCmdBuildInfo())

	xgressEdgeFactory := xgress_edge.NewFactory(config, r, r.GetStateManager())
	xgress.GlobalRegistry().Register(common.EdgeBinding, xgressEdgeFactory)
	if err := r.RegisterXrctrl(xgressEdgeFactory); err != nil {
		logrus.WithError(err).Panic("error registering edge in framework")
	}

	xgressEdgeTransportFactory := xgress_edge_transport.NewFactory()
	xgress.GlobalRegistry().Register(xgress_edge_transport.BindingName, xgressEdgeTransportFactory)

	xgressEdgeTunnelFactory := xgress_edge_tunnel.NewFactory(r, config, r.GetStateManager())
	xgress.GlobalRegistry().Register(common.TunnelBinding, xgressEdgeTunnelFactory)
	if err := r.RegisterXrctrl(xgressEdgeTunnelFactory); err != nil {
		logrus.WithError(err).Panic("error registering edge tunnel in framework")
	}

	if err := r.RegisterXrctrl(r.GetStateManager()); err != nil {
		logrus.WithError(err).Panic("error registering state manager in framework")
	}

	if self.CliAgentEnabled {
		options := agent.Options{
			Addr:       self.CliAgentAddr,
			AppId:      config.Id.Token,
			AppType:    "router",
			AppVersion: version.GetVersion(),
			AppAlias:   self.CliAgentAlias,
		}
		if config.EnableDebugOps {
			self.EnableDebugOps = true
		}
		r.RegisterDefaultAgentOps(self.EnableDebugOps)
		debugops.RegisterEdgeRouterAgentOps(r, self.EnableDebugOps)

		options.CustomOps = map[byte]func(conn net.Conn) error{
			agent.CustomOp:      r.HandleAgentOp,
			agent.CustomOpAsync: r.HandleAgentAsyncOp,
		}

		if err := agent.Listen(options); err != nil {
			pfxlog.Logger().WithError(err).Error("unable to start CLI agent")
		}
	}

	go self.waitForShutdown(r)

	if err = r.Run(); err != nil {
		logrus.WithError(err).Fatal("error starting")
	}
}

func (self *RouterAction) waitForShutdown(r *router.Router) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGQUIT, syscall.SIGINT, syscall.SIGTERM)

	s := <-ch

	if s == syscall.SIGQUIT {
		fmt.Println("=== STACK DUMP BEGIN ===")
		debugz.DumpStack()
		fmt.Println("=== STACK DUMP CLOSE ===")
	}

	pfxlog.Logger().Info("shutting down ziti router")

	if err := r.Shutdown(); err != nil {
		pfxlog.Logger().WithError(err).Info("error encountered during shutdown")
	}
}
