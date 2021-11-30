//go:build all

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

package main

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/build"
	"github.com/openziti/ziti/common/version"
	"github.com/openziti/ziti/ziti-ops/logs"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var verbose bool
var logFormatter string

func init() {
	options := pfxlog.DefaultOptions().SetTrimPrefix("github.com/openziti/").NoColor()
	pfxlog.GlobalInit(logrus.InfoLevel, options)
	build.InitBuildInfo(version.GetCmdBuildInfo())

	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")
	root.PersistentFlags().StringVar(&logFormatter, "log-formatter", "", "Specify log formatter [json|pfxlog|text]")

	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Show component version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version.GetBuildMetadata(verbose))
		},
	})

	root.AddCommand(logs.NewLogsCommand())
}

var root = &cobra.Command{
	Use:   "ziti-ops",
	Short: "Ziti Ops Tools",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if verbose {
			logrus.SetLevel(logrus.DebugLevel)
		}

		switch logFormatter {
		case "pfxlog":
			pfxlog.SetFormatter(pfxlog.NewFormatter(pfxlog.DefaultOptions().SetTrimPrefix("github.com/openziti/").StartingToday()))
		case "json":
			pfxlog.SetFormatter(&logrus.JSONFormatter{TimestampFormat: "2006-01-02T15:04:05.000Z"})
		case "text":
			pfxlog.SetFormatter(&logrus.TextFormatter{})
		default:
			// let logrus do its own thing
		}
	},
}

func main() {
	if err := root.Execute(); err != nil {
		fmt.Printf("error: %s\n", err)
	}
}
