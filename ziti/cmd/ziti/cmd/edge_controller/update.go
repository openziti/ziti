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
	"github.com/Jeffail/gabs"
	cmdutil "github.com/openziti/ziti/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/ziti/util"
	"github.com/spf13/cobra"
	"gopkg.in/resty.v1"
	"io"
)

// newUpdateCmd creates a command object for the "controller update" command
func newUpdateCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "updates various entities managed by the Ziti Edge Controller",
		Long:  "updates various entities managed by the Ziti Edge Controller",
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			cmdhelper.CheckErr(err)
		},
	}

	cmd.AddCommand(newUpdateAuthenticatorCmd(f, out, errOut))
	cmd.AddCommand(newUpdateConfigCmd(f, out, errOut))
	cmd.AddCommand(newUpdateCaCmd(f, out, errOut))
	cmd.AddCommand(newUpdateEdgeRouterCmd(f, out, errOut))
	cmd.AddCommand(newUpdateEdgeRouterPolicyCmd(f, out, errOut))
	cmd.AddCommand(newUpdateIdentityCmd(f, out, errOut))
	cmd.AddCommand(newUpdateIdentityConfigsCmd(f, out, errOut))
	cmd.AddCommand(newUpdateServiceCmd(f, out, errOut))
	cmd.AddCommand(newUpdateServicePolicyCmd(f, out, errOut))
	cmd.AddCommand(newUpdateServiceEdgeRouterPolicyCmd(f, out, errOut))
	cmd.AddCommand(newUpdateTerminatorCmd(f, out, errOut))
	cmd.AddCommand(newUpdatePostureCheckCmd(f, out, errOut))

	return cmd
}

func putEntityOfType(entityType string, body string, options *commonOptions) (*gabs.Container, error) {
	return updateEntityOfType(entityType, body, options, resty.MethodPut)
}

func patchEntityOfType(entityType string, body string, options *commonOptions) (*gabs.Container, error) {
	return updateEntityOfType(entityType, body, options, resty.MethodPatch)
}

func postEntityOfType(entityType string, body string, options *commonOptions) (*gabs.Container, error) {
	return updateEntityOfType(entityType, body, options, resty.MethodPost)
}

func deleteEntityOfTypeWithBody(entityType string, body string, options *commonOptions) (*gabs.Container, error) {
	return updateEntityOfType(entityType, body, options, resty.MethodDelete)
}

// updateEntityOfType updates an entity of the given type on the Ziti Edge Controller
func updateEntityOfType(entityType string, body string, options *commonOptions, method string) (*gabs.Container, error) {
	return util.EdgeControllerUpdate(entityType, body, options.Out, method, options.OutputJSONRequest, options.OutputJSONResponse)
}

func doRequest(entityType string, options *commonOptions, doRequest func(request *resty.Request, url string) (*resty.Response, error)) (*gabs.Container, error) {
	return util.EdgeControllerRequest(entityType, options.Out, options.OutputJSONResponse, doRequest)
}
