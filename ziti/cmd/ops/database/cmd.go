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

package database

import (
	"fmt"
	"github.com/openziti/ziti-db-explorer/cmd/ziti-db-explorer/zdecli"
	"github.com/openziti/ziti/ziti/util"
	"github.com/spf13/cobra"
	"io"
)

func NewCmdDb(out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := util.NewEmptyParentCmd("db", "Interact with Ziti database files")

	exploreCmd := &cobra.Command{
		Use:   "explore <ctrl.db>|help|version",
		Short: "Interactive CLI to explore Ziti database files",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := zdecli.Run("ziti db explore", args[0]); err != nil {
				_, _ = errOut.Write([]byte(fmt.Sprintf("Error: %s", err)))
			}
		},
	}

	cmd.AddCommand(exploreCmd)
	cmd.AddCommand(NewCompactAction())
	cmd.AddCommand(NewDiskUsageAction())
	cmd.AddCommand(NewAddDebugAdminAction())
	cmd.AddCommand(NewAnonymizeAction())
	cmd.AddCommand(NewDeleteSessionsFromDbCmd())

	return cmd
}
