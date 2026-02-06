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

package fabric

import (
	"github.com/Jeffail/gabs"
	"github.com/openziti/edge-api/rest_management_api_client"
	restClient "github.com/openziti/ziti/v2/controller/rest_client"
	"github.com/openziti/ziti/v2/ziti/cmd/api"
	"github.com/openziti/ziti/v2/ziti/cmd/common"
	"github.com/openziti/ziti/v2/ziti/util"
	"github.com/spf13/cobra"
	"gopkg.in/resty.v1"
)

// NewFabricCmd creates a command object for the fabric command
func NewFabricCmd(p common.OptionsProvider) *cobra.Command {
	fabricCmd := util.NewEmptyParentCmd("fabric", "Manage the Fabric components of a Ziti network using the Ziti Fabric REST and WebSocket APIs")
	fabricCmd.AddCommand(newCreateCommand(p), newListCmd(p), newUpdateCommand(p), newDeleteCmd(p))
	fabricCmd.AddCommand(NewInspectCmd(p))
	fabricCmd.AddCommand(newValidateCommand(p))
	fabricCmd.AddCommand(newDbCmd(p))
	fabricCmd.AddCommand(NewStreamCommand(p))
	return fabricCmd
}

func newCreateCommand(p common.OptionsProvider) *cobra.Command {
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "creates various entities managed by the Ziti Controller",
	}

	AddCreateCommands(createCmd, p)

	return createCmd
}

// AddCreateCommands adds all fabric create subcommands to the given parent command
func AddCreateCommands(cmd *cobra.Command, p common.OptionsProvider) {
	cmd.AddCommand(newCreateRouterCmd(p))
	cmd.AddCommand(newCreateServiceCmd(p))
	cmd.AddCommand(newCreateTerminatorCmd(p))
}

// AddCreateCommandsConsolidated adds fabric create subcommands for consolidated top-level use
// - router: prefixed with fabric- and hidden (edge-router is preferred)
// - service: prefixed with fabric- and hidden (edge service is preferred)
// - terminator: no prefix, this is the default terminator command
func AddCreateCommandsConsolidated(cmd *cobra.Command, p common.OptionsProvider) {
	routerCmd := newCreateRouterCmd(p)
	routerCmd.Use = "fabric-router <path-to-cert>"
	routerCmd.Hidden = true
	cmd.AddCommand(routerCmd)

	serviceCmd := newCreateServiceCmd(p)
	serviceCmd.Use = "fabric-service <name>"
	serviceCmd.Hidden = true
	cmd.AddCommand(serviceCmd)

	// Terminator is the default - no prefix
	cmd.AddCommand(newCreateTerminatorCmd(p))
}

func newUpdateCommand(p common.OptionsProvider) *cobra.Command {
	updateCmd := &cobra.Command{
		Use:   "update",
		Short: "update various entities managed by the Ziti Controller",
	}

	AddUpdateCommands(updateCmd, p)

	return updateCmd
}

// AddUpdateCommands adds all fabric update subcommands to the given parent command
func AddUpdateCommands(cmd *cobra.Command, p common.OptionsProvider) {
	cmd.AddCommand(newUpdateLinkCmd(p))
	cmd.AddCommand(newUpdateRouterCmd(p))
	cmd.AddCommand(newUpdateServiceCmd(p))
	cmd.AddCommand(newUpdateTerminatorCmd(p))
}

// AddUpdateCommandsConsolidated adds fabric update subcommands for consolidated top-level use
func AddUpdateCommandsConsolidated(cmd *cobra.Command, p common.OptionsProvider) {
	// link is fabric-only, no prefix needed
	cmd.AddCommand(newUpdateLinkCmd(p))

	routerCmd := newUpdateRouterCmd(p)
	routerCmd.Use = "fabric-" + routerCmd.Use
	routerCmd.Hidden = true
	cmd.AddCommand(routerCmd)

	serviceCmd := newUpdateServiceCmd(p)
	serviceCmd.Use = "fabric-" + serviceCmd.Use
	serviceCmd.Hidden = true
	cmd.AddCommand(serviceCmd)

	// Terminator is the default - no prefix
	cmd.AddCommand(newUpdateTerminatorCmd(p))
}

func NewStreamCommand(p common.OptionsProvider) *cobra.Command {
	streamCmd := &cobra.Command{
		Use:   "stream",
		Short: "stream fabric operational data",
	}

	streamCmd.AddCommand(NewStreamEventsCmd(p))
	streamTracesCmd := NewStreamTracesCmd(p)
	streamCmd.AddCommand(streamTracesCmd)

	var toggleTracesCmd = &cobra.Command{
		Use:   "toggle",
		Short: "Toggle traces on or off",
	}
	streamTracesCmd.AddCommand(toggleTracesCmd)
	toggleTracesCmd.AddCommand(NewStreamTogglePipeTracesCmd(p))

	return streamCmd
}

func newValidateCommand(p common.OptionsProvider) *cobra.Command {
	validateCmd := &cobra.Command{
		Use:   "validate",
		Short: "validate model data",
	}

	validateCmd.AddCommand(NewValidateCircuitsCmd(p))
	validateCmd.AddCommand(NewValidateTerminatorsCmd(p))
	validateCmd.AddCommand(NewValidateRouterLinksCmd(p))
	validateCmd.AddCommand(NewValidateRouterSdkTerminatorsCmd(p))
	validateCmd.AddCommand(NewValidateRouterErtTerminatorsCmd(p))
	validateCmd.AddCommand(NewValidateRouterDataModelCmd(p))
	validateCmd.AddCommand(NewValidateIdentityConnectionStatusesCmd(p))
	return validateCmd
}

// createEntityOfType create an entity of the given type on the Ziti Controller
func createEntityOfType(entityType string, body string, options *api.Options) (*gabs.Container, error) {
	return util.ControllerCreate("fabric", entityType, body, options.Out, options.OutputJSONRequest, options.OutputJSONResponse, options.Timeout, options.Verbose)
}

func patchEntityOfType(entityType string, body string, options *api.Options) (*gabs.Container, error) {
	return updateEntityOfType(entityType, body, options, resty.MethodPatch)
}

// updateEntityOfType updates an entity of the given type on the Ziti Edge Controller
func updateEntityOfType(entityType string, body string, options *api.Options, method string) (*gabs.Container, error) {
	return util.ControllerUpdate(util.FabricAPI, entityType, body, options.Out, method, options.OutputJSONRequest, options.OutputJSONResponse, options.Timeout, options.Verbose)
}

func WithFabricClient(clientOpts util.ClientOpts, f func(client *restClient.ZitiFabric) error) error {
	client, err := util.NewFabricManagementClient(clientOpts)
	if err != nil {
		return err
	}
	return f(client)
}

func WithEdgeClient(clientOpts util.ClientOpts, f func(client *rest_management_api_client.ZitiEdgeManagement) error) error {
	client, err := util.NewEdgeManagementClient(clientOpts)
	if err != nil {
		return err
	}
	return f(client)
}
