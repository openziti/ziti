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

package subcmd

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
)

func init() {
	Root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")
}

var Root = &cobra.Command{
	Use:   filepath.Base(os.Args[0]),
	Short: "Ziti Fabric Toolbox",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if verbose {
			logrus.SetLevel(logrus.DebugLevel)
		}
	},
}
var verbose bool

func Execute() {
	if err := Root.Execute(); err != nil {
		println(fmt.Errorf("error: %s", err))
	}
}
