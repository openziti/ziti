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

	"github.com/openziti/ziti/common/version"
	"github.com/openziti/ziti/router"
	"github.com/openziti/ziti/router/env"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func NewRunRouterCmd() *cobra.Command {
	action := &RouterAction{}
	return NewCustomRunRouterCommand("router", "Start the router with the given configuration", action)
}

func NewCustomRunRouterCommand(name string, desc string, action *RouterAction) *cobra.Command {
	var cmd = &cobra.Command{
		Use:    fmt.Sprintf("%s <config>", name),
		Short:  desc,
		Args:   cobra.ExactArgs(1),
		Run:    action.Run,
		PreRun: action.PreRun,
	}

	action.BindFlags(cmd)

	//flags are added to an internal map and read later on, see getFlags()
	cmd.Flags().BoolVar(&action.EnableDebugOps, "debug-ops", false, "Enable/disable debug agent operations (disabled by default)")
	cmd.Flags().BoolVarP(&action.ForceCertificateExtension, "extend", "e", false, "force extension of enrollment certificates on startup")
	return cmd

}

type RouterAction struct {
	Options
	EnableDebugOps            bool
	ForceCertificateExtension bool
}

func (self *RouterAction) Run(cmd *cobra.Command, args []string) {
	startLogger := logrus.WithField("version", version.GetVersion()).
		WithField("go-version", version.GetGoVersion()).
		WithField("os", version.GetOS()).
		WithField("arch", version.GetArchitecture()).
		WithField("build-date", version.GetBuildDate()).
		WithField("revision", version.GetRevision()).
		WithField("configFile", args[0])

	config, err := env.LoadConfig(args[0])
	if err != nil {
		startLogger.WithError(err).Error("error loading ziti router config")
		panic(err)
	}

	config.Edge.ForceExtendEnrollment = self.ForceCertificateExtension

	startLogger = startLogger.WithField("routerId", config.Id.Token)
	startLogger.Info("starting ziti router")

	r := router.Create(config, version.GetCmdBuildInfo())

	if self.CliAgentEnabled {
		if self.EnableDebugOps {
			config.EnableDebugOps = true
		}

		r.RunCliAgent(self.CliAgentAddr, self.CliAgentAlias)
	}

	go r.ListenForShutdownSignal()

	if err = r.Run(); err != nil {
		logrus.WithError(err).Fatal("error starting")
	}
}
