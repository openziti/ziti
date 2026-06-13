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
	"io"
	"log/slog"
	"os"

	"github.com/openziti/ziti/v2/common/logging"
	"github.com/openziti/ziti/v2/ziti/tunnel"
	"github.com/openziti/ziti/v2/ziti/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
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
	cmd.PersistentFlags().StringVar(&self.CliAgentAddr, "cli-agent-addr", "", "Specify where CLI Agent should listen (ex: unix:/tmp/myfile.sock or tcp:127.0.0.1:10001)")
	cmd.PersistentFlags().StringVar(&self.CliAgentAlias, "cli-agent-alias", "", "Alias which can be used by ziti agent commands to find this instance")
	logging.AddFlags(cmd.PersistentFlags())
}

func (self *Options) PreRun(cmd *cobra.Command, _ []string) {
	// Install the slog handler chain before anything else can log: this wires
	// logrus into the bridge and seeds the default Registry so the agent
	// callbacks (registered later in Run) have a Registry to drive.
	//
	// --verbose and --log-formatter are defined both here and on the alias
	// parent (ziti controller / ziti router), so read them across the whole
	// command chain rather than from self.* alone. That honors the flag
	// wherever it appears - "ziti controller --verbose run ..." as well as
	// "ziti controller run --verbose ..." - independent of cobra's rules for
	// which scope a duplicated flag binds to.
	verbose := self.Verbose
	if v, ok := changedFlagValue(cmd, "verbose"); ok {
		verbose = v == "true"
	}
	logFormatter := self.LogFormatter
	if v, ok := changedFlagValue(cmd, "log-formatter"); ok {
		logFormatter = v
	}

	asyncOpts, err := logging.OptionsFromFlags(cmd.Flags())
	if err != nil {
		logrus.WithError(err).Fatal("invalid --log-* flags")
	}
	handler, err := logging.BuildHandlerForFormat(os.Stderr, asyncOpts, logFormatter)
	if err != nil {
		logrus.WithError(err).Fatal("unable to build log handler")
	}
	initialLevel := slog.LevelInfo
	if verbose {
		initialLevel = slog.LevelDebug
	}
	logging.Install(handler, initialLevel)

	util.LogReleaseVersionCheck()
}

// changedFlagValue walks cmd and its ancestors for a persistent flag named
// `name`, returning the value of the first one that was explicitly set. The run
// command and its alias parent both define --verbose / --log-formatter; which
// one a given occurrence binds to depends on its position and cobra internals,
// so reading across the chain lets PreRun honor the flag wherever it appears.
func changedFlagValue(cmd *cobra.Command, name string) (string, bool) {
	for c := cmd; c != nil; c = c.Parent() {
		if f := c.PersistentFlags().Lookup(name); f != nil && f.Changed {
			return f.Value.String(), true
		}
	}
	return "", false
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
