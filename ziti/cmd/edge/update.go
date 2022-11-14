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
	"github.com/Jeffail/gabs"
	"github.com/openziti/ziti/ziti/cmd/api"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/util"
	"github.com/spf13/cobra"
	"gopkg.in/resty.v1"
	"io"
)

// newUpdateCmd creates a command object for the "controller update" command
func newUpdateCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "updates various entities managed by the Ziti Edge Controller",
		Long:  "updates various entities managed by the Ziti Edge Controller",
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			cmdhelper.CheckErr(err)
		},
	}

	cmd.AddCommand(newUpdateAuthenticatorCmd(out, errOut))
	cmd.AddCommand(newUpdateConfigCmd(out, errOut))
	cmd.AddCommand(newUpdateConfigTypeCmd(out, errOut))
	cmd.AddCommand(newUpdateCaCmd(out, errOut))
	cmd.AddCommand(newUpdateEdgeRouterCmd(out, errOut))
	cmd.AddCommand(newUpdateEdgeRouterPolicyCmd(out, errOut))
	cmd.AddCommand(newUpdateIdentityCmd(out, errOut))
	cmd.AddCommand(newUpdateIdentityConfigsCmd(out, errOut))
	cmd.AddCommand(newUpdateServiceCmd(out, errOut))
	cmd.AddCommand(newUpdateServicePolicyCmd(out, errOut))
	cmd.AddCommand(newUpdateServiceEdgeRouterPolicyCmd(out, errOut))
	cmd.AddCommand(newUpdateTerminatorCmd(out, errOut))
	cmd.AddCommand(newUpdatePostureCheckCmd(out, errOut))

	return cmd
}

func putEntityOfType(entityType string, body string, options *api.Options) (*gabs.Container, error) {
	return updateEntityOfType(entityType, body, options, resty.MethodPut)
}

func patchEntityOfType(entityType string, body string, options *api.Options) (*gabs.Container, error) {
	return updateEntityOfType(entityType, body, options, resty.MethodPatch)
}

func postEntityOfType(entityType string, body string, options *api.Options) (*gabs.Container, error) {
	return updateEntityOfType(entityType, body, options, resty.MethodPost)
}

func deleteEntityOfTypeWithBody(entityType string, body string, options *api.Options) (*gabs.Container, error) {
	return updateEntityOfType(entityType, body, options, resty.MethodDelete)
}

// updateEntityOfType updates an entity of the given type on the Ziti Edge Controller
func updateEntityOfType(entityType string, body string, options *api.Options, method string) (*gabs.Container, error) {
	return util.ControllerUpdate(util.EdgeAPI, entityType, body, options.Out, method, options.OutputJSONRequest, options.OutputJSONResponse, options.Timeout, options.Verbose)
}

func doRequest(entityType string, options *api.Options, doRequest func(request *resty.Request, url string) (*resty.Response, error)) (*gabs.Container, error) {
	return util.EdgeControllerRequest(entityType, options.Out, options.OutputJSONResponse, options.Timeout, options.Verbose, doRequest)
}
