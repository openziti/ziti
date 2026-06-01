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
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/openziti/ziti/v2/controller/config"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/v2/common/agent"
	"github.com/openziti/ziti/v2/common/agentlog"
	"github.com/openziti/ziti/v2/common/logging"
	"github.com/openziti/ziti/v2/common/version"
	"github.com/openziti/ziti/v2/controller"
	"github.com/openziti/ziti/v2/controller/server"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func formatRootCause(err error) string {
	if err == nil {
		return ""
	}

	// Prefer ApiError cause if present
	var apiErr *errorz.ApiError
	if errors.As(err, &apiErr) {
		if apiErr.Cause != nil {
			// apiErr may wrap a generic message; print the cause chain
			return fmt.Sprintf("%v", apiErr.Cause)
		}
		return fmt.Sprintf("%v", apiErr)
	}

	// Fall back to deepest unwrap
	root := err
	for {
		next := errors.Unwrap(root)
		if next == nil {
			break
		}
		root = next
	}
	if root == err {
		return fmt.Sprintf("%v", err)
	}
	return fmt.Sprintf("%v (root cause: %v)", err, root)
}

// versionAttrs returns the build/version fields the controller and router
// startup loggers carry, as slog attrs for the logging.Fatal hard-exit paths.
// A fresh slice is returned on each call so callers can append safely.
func versionAttrs() []slog.Attr {
	return []slog.Attr{
		slog.String("version", version.GetVersion()),
		slog.String("go-version", version.GetGoVersion()),
		slog.String("os", version.GetOS()),
		slog.String("arch", version.GetArchitecture()),
		slog.String("build-date", version.GetBuildDate()),
		slog.String("revision", version.GetRevision()),
	}
}

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

	if delaySecondsStr := os.Getenv("DELAY_START_SECONDS"); delaySecondsStr != "" {
		delaySeconds, err := strconv.Atoi(delaySecondsStr)
		if err != nil {
			pfxlog.Logger().WithError(err).Error("could not parse DELAY_START_SECONDS, using default value of 10s")
			delaySeconds = 10
		} else if delaySeconds < 1 || delaySeconds > 120 {
			pfxlog.Logger().Errorf("invalid value '%d' for DELAY_START_SECONDS, using default value of 10s", delaySeconds)
		}

		pfxlog.Logger().Infof("delaying start-up for %d seconds", delaySeconds)
		time.Sleep(time.Duration(delaySeconds) * time.Second)
	}

	ctrlConfig, err := config.LoadConfig(args[0])
	if err != nil {
		logging.Fatal(cmd.Context(), "error starting ziti-controller",
			append(versionAttrs(), slog.String("error", err.Error()))...)
	}

	startLogger = startLogger.WithField("nodeId", ctrlConfig.Id.Token)
	startLogger.Info("starting ziti-controller")

	if self.fabricController, err = controller.NewController(ctrlConfig, version.GetCmdBuildInfo()); err != nil {
		cause := formatRootCause(err)
		logging.Fatal(cmd.Context(), "unable to create fabric controller",
			append(versionAttrs(),
				slog.String("nodeId", ctrlConfig.Id.Token),
				slog.String("cause", cause),
				slog.String("error", err.Error()))...)
	}

	self.edgeController, err = server.NewController(self.fabricController)
	if err != nil {
		cause := formatRootCause(err)
		logging.Fatal(cmd.Context(), "unable to create edge controller",
			append(versionAttrs(),
				slog.String("nodeId", ctrlConfig.Id.Token),
				slog.String("cause", cause),
				slog.String("error", err.Error()))...)
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
		if err := agent.RegisterLogLevelHandlers(agentlog.DefaultLogLevelCallbacks()); err != nil {
			pfxlog.Logger().WithError(err).Error("unable to register agent log-level handlers")
		}
		if err := agent.Listen(options); err != nil {
			pfxlog.Logger().WithError(err).Error("unable to start CLI agent")
		}
	}

	go self.waitForShutdown(cmd.Context())

	self.edgeController.Run()
	if err := self.fabricController.Run(); err != nil {
		cause := formatRootCause(err)
		logging.Fatal(cmd.Context(), "fabric controller exited with error",
			append(versionAttrs(),
				slog.String("nodeId", ctrlConfig.Id.Token),
				slog.String("cause", cause),
				slog.String("error", err.Error()))...)
	}
}

func (self *ControllerAction) waitForShutdown(ctx context.Context) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(ch)

	pfxlog.Logger().Info("waiting for shutdown signal or context cancel")

	select {
	case sig := <-ch:
		pfxlog.Logger().Infof("received signal: %v", sig)
	case <-ctx.Done():
		pfxlog.Logger().Info("context cancelled, shutting down")
	}

	self.edgeController.Shutdown()
	self.fabricController.Shutdown()

	pfxlog.Logger().Info("shutdown complete")
}
