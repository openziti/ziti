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

package edge_controller

import (
	"io"

	"github.com/Jeffail/gabs"
	cmdutil "github.com/openziti/ziti/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/ziti/util"
	"github.com/spf13/cobra"
)

// newListCmd creates a command object for the "controller list" command
func newCreateCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "creates various entities managed by the Ziti Edge Controller",
		Long:  "Creates various entities managed by the Ziti Edge Controller",
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			cmdhelper.CheckErr(err)
		},
	}

	cmd.AddCommand(newCreateAuthenticatorCmd(f, out, errOut))
	cmd.AddCommand(newCreateCaCmd(f, out, errOut))
	cmd.AddCommand(newCreateConfigCmd(f, out, errOut))
	cmd.AddCommand(newCreateConfigTypeCmd(f, out, errOut))
	cmd.AddCommand(newCreateEdgeRouterCmd(f, out, errOut))
	cmd.AddCommand(newCreateEdgeRouterPolicyCmd(f, out, errOut))
	cmd.AddCommand(newCreateTerminatorCmd(f, out, errOut))
	cmd.AddCommand(newCreateIdentityCmd(f, out, errOut))
	cmd.AddCommand(newCreateServiceCmd(f, out, errOut))
	cmd.AddCommand(newCreateServiceEdgeRouterPolicyCmd(f, out, errOut))
	cmd.AddCommand(newCreateServicePolicyCmd(f, out, errOut))
	cmd.AddCommand(newCreatePostureCheckCmd(f, out, errOut))

	return cmd
}

// createEntityOfType create an entity of the given type on the Ziti Edge Controller
func createEntityOfType(entityType string, body string, options *commonOptions) (*gabs.Container, error) {
	return util.EdgeControllerCreate(entityType, body, options.Out, options.OutputJSONResponse)
}
