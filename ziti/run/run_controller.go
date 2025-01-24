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
	"github.com/openziti/ziti/controller/config"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/agent"
	"github.com/openziti/ziti/common/version"
	"github.com/openziti/ziti/controller"
	"github.com/openziti/ziti/controller/server"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func NewRunControllerCmd() *cobra.Command {
	action := &ControllerAction{}

	cmd := &cobra.Command{
		Use:    "controller <config>",
		Short:  "Run an OpenZiti controller with the given configuration",
		Args:   cobra.ExactArgs(1),
		Run:    action.Run,
		PreRun: action.PreRun,
	}

	action.BindFlags(cmd)
	return cmd
}

type ControllerAction struct {
	Options
	fabricController *controller.Controller
	edgeController   *server.Controller
}

func (self *ControllerAction) Run(cmd *cobra.Command, args []string) {
	startLogger :=
		logrus.WithField("version", version.GetVersion()).
			WithField("go-version", version.GetGoVersion()).
			WithField("os", version.GetOS()).
			WithField("arch", version.GetArchitecture()).
			WithField("build-date", version.GetBuildDate()).
			WithField("revision", version.GetRevision())

	ctrlConfig, err := config.LoadConfig(args[0])
	if err != nil {
		startLogger.WithError(err).Error("error starting ziti-controller")
		panic(err)
	}

	startLogger = startLogger.WithField("nodeId", ctrlConfig.Id.Token)
	startLogger.Info("starting ziti-controller")

	if self.fabricController, err = controller.NewController(ctrlConfig, version.GetCmdBuildInfo()); err != nil {
		fmt.Printf("unable to create fabric controller %+v\n", err)
		panic(err)
	}

	self.edgeController, err = server.NewController(self.fabricController)
	if err != nil {
		panic(err)
	}

	self.edgeController.Initialize()

	if self.CliAgentEnabled {
		options := agent.Options{
			Addr:       self.CliAgentAddr,
			AppId:      ctrlConfig.Id.Token,
			AppType:    "controller",
			AppVersion: version.GetVersion(),
			AppAlias:   self.CliAgentAlias,
		}
		options.CustomOps = map[byte]func(conn net.Conn) error{
			agent.CustomOpAsync: self.fabricController.HandleCustomAgentAsyncOp,
		}
		if err := agent.Listen(options); err != nil {
			pfxlog.Logger().WithError(err).Error("unable to start CLI agent")
		}
	}

	go self.waitForShutdown()

	self.edgeController.Run()
	if err := self.fabricController.Run(); err != nil {
		panic(err)
	}
}

func (self *ControllerAction) waitForShutdown() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)

	<-ch

	pfxlog.Logger().Info("shutting down ziti-controller")
	self.edgeController.Shutdown()
	self.fabricController.Shutdown()
}
