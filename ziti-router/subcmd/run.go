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
	"github.com/netfoundry/ziti-edge/gateway/xgress_edge"
	"github.com/netfoundry/ziti-fabric/router"
	"github.com/netfoundry/ziti-fabric/router/xgress"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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
	if config, err := router.LoadConfig(args[0]); err == nil {
		config.SetFlags(getFlags(cmd))

		r := router.Create(config)

		xgressEdgeFactory := xgress_edge.NewFactory()
		xgress.GlobalRegistry().Register("edge", xgressEdgeFactory)
		if err := r.RegisterXctrl(xgressEdgeFactory); err != nil {
			logrus.Panicf("error registering edge in framework (%v)", err)
		}

		if err := r.Run(); err != nil {
			logrus.Panicf("error starting (%v)", err)
		}

	} else {
		logrus.Panicf("error loading configuration (%v)", err)
	}
}

func getFlags(cmd *cobra.Command) map[string]*pflag.Flag {
	ret := map[string]*pflag.Flag{}
	cmd.Flags().Visit(func(f *pflag.Flag) {
		ret[f.Name] = f
	})
	return ret
}
