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
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/common/enrollment"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func init() {
	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")
	root.PersistentFlags().StringVar(&logFormatter, "log-formatter", "", "Specify log formatter [json|pfxlog|text]")

	root.AddCommand(enrollment.NewEnrollCommand())
}

var root = &cobra.Command{
	Use:   "ziti-probe",
	Short: "Ziti Probe",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if verbose {
			logrus.SetLevel(logrus.DebugLevel)
		}

		switch logFormatter {
		case "pfxlog":
			logrus.SetFormatter(pfxlog.NewFormatter(pfxlog.DefaultOptions().StartingToday()))
		case "json":
			logrus.SetFormatter(&logrus.JSONFormatter{TimestampFormat: "2006-01-02T15:04:05.000Z"})
		case "text":
			logrus.SetFormatter(&logrus.TextFormatter{})
		default:
			// let logrus do its own thing
		}

	},
}

var verbose bool
var logFormatter string

func Execute() {
	if err := root.Execute(); err != nil {
		fmt.Printf("error: %s\n", err)
	}
}
