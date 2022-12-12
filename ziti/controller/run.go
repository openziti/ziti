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

package controller

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/agent"
	"github.com/openziti/edge/controller/server"
	"github.com/openziti/fabric/controller"
	"github.com/openziti/ziti/common/version"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func NewRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run <config>",
		Short: "Run controller configuration",
		Args:  cobra.ExactArgs(1),
		Run:   run,
	}
}

func run(cmd *cobra.Command, args []string) {
	startLogger :=
		logrus.WithField("version", version.GetVersion()).
			WithField("go-version", version.GetGoVersion()).
			WithField("os", version.GetOS()).
			WithField("arch", version.GetArchitecture()).
			WithField("build-date", version.GetBuildDate()).
			WithField("revision", version.GetRevision())

	config, err := controller.LoadConfig(args[0])
	if err != nil {
		startLogger.WithError(err).Error("error starting ziti-controller")
		panic(err)
	}
	config.SyncRaftToDb = syncRaftToDb

	startLogger = startLogger.WithField("nodeId", config.Id.Token)
	startLogger.Info("starting ziti-controller")

	var fabricController *controller.Controller
	if fabricController, err = controller.NewController(config, version.GetCmdBuildInfo()); err != nil {
		fmt.Printf("unable to create fabric controller %+v\n", err)
		panic(err)
	}

	edgeController, err := server.NewController(config, fabricController)

	if err != nil {
		panic(err)
	}

	edgeController.Initialize()

	if cliAgentEnabled {
		options := agent.Options{Addr: cliAgentAddr}
		options.CustomOps = map[byte]func(conn net.Conn) error{
			agent.CustomOp:      fabricController.HandleCustomAgentOp,
			agent.CustomOpAsync: fabricController.HandleCustomAgentAsyncOp,
		}
		if err := agent.Listen(options); err != nil {
			pfxlog.Logger().WithError(err).Error("unable to start CLI agent")
		}
	}

	go waitForShutdown(fabricController, edgeController)
	edgeController.Run()
	if err := fabricController.Run(); err != nil {
		panic(err)
	}
}

func waitForShutdown(fabricController *controller.Controller, edgeController *server.Controller) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)

	<-ch

	pfxlog.Logger().Info("shutting down ziti-controller")
	edgeController.Shutdown()
	fabricController.Shutdown()
}
