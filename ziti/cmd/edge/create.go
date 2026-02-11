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
	"io"

	"github.com/Jeffail/gabs"
	"github.com/openziti/ziti/v2/ziti/cmd/api"
	"github.com/openziti/ziti/v2/ziti/util"

	"github.com/spf13/cobra"
)

func Ptr[T any](sub T) *T {
	return &sub
}

// newCreateCmd creates a command object for the "create" command
func newCreateCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "creates various entities managed by the Ziti Edge Controller",
		Long:  "Creates various entities managed by the Ziti Edge Controller",
	}

	AddCreateCommands(cmd, out, errOut)

	return cmd
}

// AddCreateCommands adds all edge create subcommands to the given parent command
func AddCreateCommands(cmd *cobra.Command, out io.Writer, errOut io.Writer) {
	addCreateCommandsInternal(cmd, out, errOut, true)
}

// AddCreateCommandsConsolidated adds edge create subcommands for consolidated top-level use
// Excludes terminator (fabric terminator is preferred)
func AddCreateCommandsConsolidated(cmd *cobra.Command, out io.Writer, errOut io.Writer) {
	addCreateCommandsInternal(cmd, out, errOut, false)
}

func addCreateCommandsInternal(cmd *cobra.Command, out io.Writer, errOut io.Writer, includeTerminator bool) {
	cmd.AddCommand(newCreateAuthenticatorCmd(out, errOut))
	cmd.AddCommand(newCreateCaCmd(out, errOut))
	cmd.AddCommand(newCreateConfigCmd(out, errOut))
	cmd.AddCommand(newCreateConfigTypeCmd(out, errOut))
	cmd.AddCommand(NewCreateEdgeRouterCmd(out, errOut))
	cmd.AddCommand(NewCreateEdgeRouterPolicyCmd(out, errOut))
	cmd.AddCommand(newCreateEnrollmentCmd(out, errOut))
	cmd.AddCommand(newCreateIdentityCmd(out, errOut))
	cmd.AddCommand(newCreatePostureCheckCmd(out, errOut))
	cmd.AddCommand(newCreateServiceCmd(out, errOut))
	cmd.AddCommand(NewCreateServiceEdgeRouterPolicyCmd(out, errOut))
	cmd.AddCommand(newCreateServicePolicyCmd(out, errOut))
	if includeTerminator {
		cmd.AddCommand(newCreateTerminatorCmd(out, errOut))
	}
	cmd.AddCommand(newCreateTransitRouterCmd(out, errOut))
	cmd.AddCommand(newCreateExtJwtSignerCmd(out, errOut))
	cmd.AddCommand(newCreateAuthPolicyCmd(out, errOut))
}

// CreateEntityOfType create an entity of the given type on the Ziti Controller
func CreateEntityOfType(entityType string, body string, options *api.Options) (*gabs.Container, error) {
	return util.ControllerCreate("edge", entityType, body, options.Out, options.OutputJSONRequest, options.OutputJSONResponse, options.Timeout, options.Verbose)
}
