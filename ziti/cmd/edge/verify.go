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

package edge

import (
	"github.com/openziti/ziti/ziti/cmd/api"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/util"
	"io"

	"github.com/spf13/cobra"
)

// newVerifyCmd creates a command object for the "controller verify" command
func newVerifyCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "verifies various entities managed by the Ziti Edge Controller",
		Long:  "Verifies various entities managed by the Ziti Edge Controller",
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			cmdhelper.CheckErr(err)
		},
	}

	cmd.AddCommand(newVerifyCaCmd(out, errOut))

	return cmd
}

// createEntityOfType create an entity of the given type on the Ziti Edge Controller
func verifyEntityOfType(entityType, body, id string, options *api.Options) error {
	return util.EdgeControllerVerify(entityType, id, body, options.Out, options.OutputJSONResponse, options.Timeout, options.Verbose)
}
