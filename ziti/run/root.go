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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/ziti/tunnel"
	"github.com/openziti/ziti/ziti/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"io"
)

type Options struct {
	Verbose      bool
	LogFormatter string

	CliAgentEnabled bool
	CliAgentAddr    string
	CliAgentAlias   string
}

func (self *Options) BindFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().BoolVarP(&self.Verbose, "verbose", "v", false, "Enable verbose logging")
	cmd.PersistentFlags().StringVar(&self.LogFormatter, "log-formatter", "", "Specify log formatter [json|pfxlog|text]")

	cmd.PersistentFlags().BoolVarP(&self.CliAgentEnabled, "cliagent", "a", true, "Enable/disabled CLI Agent (enabled by default)")
	cmd.PersistentFlags().StringVar(&self.CliAgentAddr, "cli-agent-addr", "", "Specify where CLI Agent should list (ex: unix:/tmp/myfile.sock or tcp:127.0.0.1:10001)")
	cmd.PersistentFlags().StringVar(&self.CliAgentAlias, "cli-agent-alias", "", "Alias which can be used by ziti agent commands to find this instance")
}

func (self *Options) PreRun(_ *cobra.Command, _ []string) {
	if self.Verbose {
		logrus.SetLevel(logrus.DebugLevel)
	}

	switch self.LogFormatter {
	case "pfxlog":
		pfxlog.SetFormatter(pfxlog.NewFormatter(pfxlog.DefaultOptions().SetTrimPrefix("github.com/openziti/").StartingToday()))
	case "json":
		pfxlog.SetFormatter(&logrus.JSONFormatter{TimestampFormat: "2006-01-02T15:04:05.000Z"})
	case "text":
		pfxlog.SetFormatter(&logrus.TextFormatter{})
	default:
		// let logrus do its own thing
	}

	util.LogReleaseVersionCheck()
}

func NewRunCmd(out, err io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run OpenZiti components, such as the controller and router",
	}

	cmd.AddCommand(NewRunControllerCmd())
	cmd.AddCommand(NewRunRouterCmd())
	cmd.AddCommand(tunnel.NewTunnelCmd(false))
	cmd.AddCommand(NewQuickStartCmd(out, err, context.Background()))

	return cmd
}
