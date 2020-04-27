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
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func init() {
	Root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")
	Root.AddCommand(createCmd)
	Root.AddCommand(listCmd)
	Root.AddCommand(getCmd)
	Root.AddCommand(setCmd)
	Root.AddCommand(streamCmd)
	Root.AddCommand(removeCmd)
}

var Root = &cobra.Command{
	Use:   "ziti-fabric",
	Short: "Ziti Fabric Management Utility",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if verbose {
			logrus.SetLevel(logrus.DebugLevel)
		}
	},
}
var verbose bool

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List fabric components",
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create fabric components",
}

var getCmd = &cobra.Command{
	Use:   "get",
	Short: "Get fabric component",
}

var setCmd = &cobra.Command{
	Use:   "set",
	Short: "Set fabric component value",
}

var removeCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove fabric component",
}

var streamCmd = &cobra.Command{
	Use:   "stream",
	Short: "Stream fabric operational data",
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update fabric components",
}

func Execute() {
	if err := Root.Execute(); err != nil {
		fmt.Printf("error: %s", err)
	}
}
