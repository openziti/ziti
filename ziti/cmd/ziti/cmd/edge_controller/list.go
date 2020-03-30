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
	"encoding/json"
	"fmt"
	"io"
	"net/url"

	"github.com/Jeffail/gabs"
	"github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/common"
	cmdutil "github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/helpers"
	"github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/util"
	"github.com/spf13/cobra"
)

// newListCmd creates a command object for the "controller list" command
func newListCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lists various entities managed by the Ziti Edge Controller",
		Long:  "Lists various entities managed by the Ziti Edge Controller",
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			cmdhelper.CheckErr(err)
		},
	}

	newOptions := func() *commonOptions {
		return &commonOptions{
			CommonOptions: common.CommonOptions{
				Factory: f,
				Out:     out,
				Err:     errOut,
			},
		}
	}

	cmd.AddCommand(newListCmdForEntityType("api-sessions", runListApiSessions, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("cas", runListCAs, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("config-types", runListConfigTypes, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("configs", runListConfigs, newOptions()))
	cmd.AddCommand(newListEdgeRoutersCmd(newOptions()))
	cmd.AddCommand(newListCmdForEntityType("edge-router-policies", runListEdgeRouterPolicies, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("terminators", runListTerminators, newOptions()))
	cmd.AddCommand(newListIdentitiesCmd(newOptions()))
	cmd.AddCommand(newListServicesCmd(newOptions()))
	cmd.AddCommand(newListCmdForEntityType("service-edge-router-policies", runListServiceEdgeRouterPolices, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("service-policies", runListServicePolices, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("sessions", runListSessions, newOptions()))

	cmd.AddCommand(newListCmdForEntityType("edge-router-role-attributes", runListEdgeRouterRoleAttributes, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("identity-role-attributes", runListIdentityRoleAttributes, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("service-role-attributes", runListServiceRoleAttributes, newOptions()))

	configTypeListRootCmd := newEntityListRootCmd("config-type")
	configTypeListRootCmd.AddCommand(newSubListCmdForEntityType("config-type", "configs", outputConfigs, newOptions()))

	edgeRouterListRootCmd := newEntityListRootCmd("edge-router")
	edgeRouterListRootCmd.AddCommand(newSubListCmdForEntityType("edge-routers", "edge-router-policies", outputEdgeRouterPolicies, newOptions()))
	edgeRouterListRootCmd.AddCommand(newSubListCmdForEntityType("edge-routers", "service-edge-router-polices", outputServiceEdgeRouterPolicies, newOptions()))
	edgeRouterListRootCmd.AddCommand(newSubListCmdForEntityType("edge-routers", "identities", outputIdentities, newOptions()))
	edgeRouterListRootCmd.AddCommand(newSubListCmdForEntityType("edge-routers", "services", outputServices, newOptions()))

	edgeRouterPolicyListRootCmd := newEntityListRootCmd("edge-router-policy")
	edgeRouterPolicyListRootCmd.AddCommand(newSubListCmdForEntityType("edge-router-policies", "edge-routers", outputEdgeRouters, newOptions()))
	edgeRouterPolicyListRootCmd.AddCommand(newSubListCmdForEntityType("edge-router-policies", "identities", outputIdentities, newOptions()))

	identityListRootCmd := newEntityListRootCmd("identity")
	identityListRootCmd.AddCommand(newSubListCmdForEntityType("identities", "edge-router-policies", outputEdgeRouterPolicies, newOptions()))
	identityListRootCmd.AddCommand(newSubListCmdForEntityType("identities", "edge-routers", outputEdgeRouters, newOptions()))
	identityListRootCmd.AddCommand(newSubListCmdForEntityType("identities", "service-policies", outputServicePolicies, newOptions()))
	identityListRootCmd.AddCommand(newSubListCmdForEntityType("identities", "services", outputServices, newOptions()))

	serviceListRootCmd := newEntityListRootCmd("service")
	serviceListRootCmd.AddCommand(newSubListCmdForEntityType("services", "configs", outputConfigs, newOptions()))
	serviceListRootCmd.AddCommand(newSubListCmdForEntityType("services", "service-policies", outputServicePolicies, newOptions()))
	serviceListRootCmd.AddCommand(newSubListCmdForEntityType("services", "service-edge-router-policies", outputServiceEdgeRouterPolicies, newOptions()))
	serviceListRootCmd.AddCommand(newSubListCmdForEntityType("services", "terminators", outputTerminators, newOptions()))
	serviceListRootCmd.AddCommand(newSubListCmdForEntityType("services", "identities", outputIdentities, newOptions()))
	serviceListRootCmd.AddCommand(newSubListCmdForEntityType("services", "edge-routers", outputEdgeRouters, newOptions()))

	serviceEdgeRouterPolicyListRootCmd := newEntityListRootCmd("service-edge-router-policy")
	serviceEdgeRouterPolicyListRootCmd.AddCommand(newSubListCmdForEntityType("service-edge-router-policies", "services", outputServices, newOptions()))
	serviceEdgeRouterPolicyListRootCmd.AddCommand(newSubListCmdForEntityType("service-edge-router-policies", "edge-routers", outputEdgeRouters, newOptions()))

	servicePolicyListRootCmd := newEntityListRootCmd("service-policy")
	servicePolicyListRootCmd.AddCommand(newSubListCmdForEntityType("service-policies", "services", outputServices, newOptions()))
	servicePolicyListRootCmd.AddCommand(newSubListCmdForEntityType("service-policies", "identities", outputIdentities, newOptions()))

	cmd.AddCommand(configTypeListRootCmd, edgeRouterListRootCmd, edgeRouterPolicyListRootCmd, identityListRootCmd, serviceListRootCmd, servicePolicyListRootCmd)

	return cmd
}

type listCommandRunner func(*commonOptions) error

type outputFunction func(o *commonOptions, children []*gabs.Container) error

func newEntityListRootCmd(entityType string) *cobra.Command {
	desc := fmt.Sprintf("list entities related to a %v instance managed by the Ziti Edge Controller", entityType)
	return &cobra.Command{
		Use:   entityType,
		Short: desc,
		Long:  desc,
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			cmdhelper.CheckErr(err)
		},
	}
}

// newListCmdForEntityType creates the list command for the given entity type
func newListCmdForEntityType(entityType string, command listCommandRunner, options *commonOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   entityType + " <filter>?",
		Short: "lists " + entityType + " managed by the Ziti Edge Controller",
		Long:  "lists " + entityType + " managed by the Ziti Edge Controller",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := command(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)

	cmd.Flags().BoolVarP(&options.OutputJSONResponse, "output-json", "j", false, "Output the full JSON response from the Ziti Edge Controller")

	return cmd
}

// newListServicesCmd creates the list command for the given entity type
func newListServicesCmd(options *commonOptions) *cobra.Command {
	var asIdentity string
	var configTypes string
	var roleFilters []string
	var roleSemantic string

	cmd := &cobra.Command{
		Use:   "services <filter>?",
		Short: "lists services managed by the Ziti Edge Controller",
		Long:  "lists services managed by the Ziti Edge Controller",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runListServices(asIdentity, configTypes, roleFilters, roleSemantic, options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)

	cmd.Flags().BoolVarP(&options.OutputJSONResponse, "output-json", "j", false, "Output the full JSON response from the Ziti Edge Controller")
	cmd.Flags().StringVar(&asIdentity, "as-identity", "", "Allow admins to see services as they would be seen by a different identity")
	cmd.Flags().StringVar(&configTypes, "config-types", "", "Override which config types to view on services")
	cmd.Flags().StringSliceVar(&roleFilters, "role-filters", nil, "Allow filtering by roles")
	cmd.Flags().StringVar(&roleSemantic, "role-semantic", "", "Specify which roles semantic to use ")

	return cmd
}

// newListEdgeRoutersCmd creates the list command for the given entity type
func newListEdgeRoutersCmd(options *commonOptions) *cobra.Command {
	var roleFilters []string
	var roleSemantic string

	cmd := &cobra.Command{
		Use:   "edge-routers <filter>?",
		Short: "lists edge routers managed by the Ziti Edge Controller",
		Long:  "lists edge routers managed by the Ziti Edge Controller",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runListEdgeRouters(roleFilters, roleSemantic, options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)

	cmd.Flags().BoolVarP(&options.OutputJSONResponse, "output-json", "j", false, "Output the full JSON response from the Ziti Edge Controller")
	cmd.Flags().StringSliceVar(&roleFilters, "role-filters", nil, "Allow filtering by roles")
	cmd.Flags().StringVar(&roleSemantic, "role-semantic", "", "Specify which roles semantic to use ")

	return cmd
}

// newListEdgeRoutersCmd creates the list command for the given entity type
func newListIdentitiesCmd(options *commonOptions) *cobra.Command {
	var roleFilters []string
	var roleSemantic string

	cmd := &cobra.Command{
		Use:   "identities <filter>?",
		Short: "lists identities managed by the Ziti Edge Controller",
		Long:  "lists identities managed by the Ziti Edge Controller",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runListIdentities(roleFilters, roleSemantic, options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)

	cmd.Flags().BoolVarP(&options.OutputJSONResponse, "output-json", "j", false, "Output the full JSON response from the Ziti Edge Controller")
	cmd.Flags().StringSliceVar(&roleFilters, "role-filters", nil, "Allow filtering by roles")
	cmd.Flags().StringVar(&roleSemantic, "role-semantic", "", "Specify which roles semantic to use ")

	return cmd
}

// newSubListCmdForEntityType creates the list command for the given entity type
func newSubListCmdForEntityType(entityType string, subType string, outputF outputFunction, options *commonOptions) *cobra.Command {
	desc := fmt.Sprintf("lists %v related to a %v instanced managed by the Ziti Edge Controller", subType, entityType)
	cmd := &cobra.Command{
		Use:   fmt.Sprintf("%v <id or name>", subType),
		Short: desc,
		Long:  desc,
		Args:  cobra.RangeArgs(1, 2),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runListChilden(entityType, subType, options, outputF)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)

	cmd.Flags().BoolVarP(&options.OutputJSONResponse, "output-json", "j", false, "Output the full JSON response from the Ziti Edge Controller")

	return cmd
}

// listEntitiesOfType queries the Ziti Controller for entities of the given type
func listEntitiesWithOptions(entityType string, options *commonOptions) ([]*gabs.Container, error) {
	params := url.Values{}
	if len(options.Args) > 0 {
		params.Add("filter", options.Args[0])
	}
	return listEntitiesOfType(entityType, params, options.OutputJSONResponse, options.Out)
}

func filterEntitiesOfType(entityType string, filter string, logJSON bool, out io.Writer) ([]*gabs.Container, error) {
	params := url.Values{}
	params.Add("filter", filter)
	return listEntitiesOfType(entityType, params, logJSON, out)
}

// listEntitiesOfType queries the Ziti Controller for entities of the given type
func listEntitiesOfType(entityType string, params url.Values, logJSON bool, out io.Writer) ([]*gabs.Container, error) {
	jsonParsed, err := util.EdgeControllerList(entityType, params, logJSON, out)

	if err != nil {
		return nil, err
	}

	return jsonParsed.S("data").Children()
}

// listEntitiesOfType queries the Ziti Controller for entities of the given type
func filterSubEntitiesOfType(entityType, subType, entityId, filter string, o *commonOptions) ([]*gabs.Container, error) {
	jsonParsed, err := util.EdgeControllerListSubEntities(entityType, subType, entityId, filter, o.OutputJSONResponse, o.Out)

	if err != nil {
		return nil, err
	}

	return jsonParsed.S("data").Children()
}

func runListEdgeRouters(roleFilters []string, roleSemantic string, options *commonOptions) error {
	params := url.Values{}
	if len(options.Args) > 0 {
		params.Add("filter", options.Args[0])
	}
	for _, roleFilter := range roleFilters {
		params.Add("roleFilter", roleFilter)
	}
	if roleSemantic != "" {
		params.Add("roleSemantic", roleSemantic)
	}
	children, err := listEntitiesOfType("edge-routers", params, options.OutputJSONResponse, options.Out)
	if err != nil {
		return err
	}

	return outputEdgeRouters(options, children)
}

func outputEdgeRouters(o *commonOptions, children []*gabs.Container) error {
	if o.OutputJSONResponse {
		return nil
	}

	for _, entity := range children {
		id, _ := entity.Path("id").Data().(string)
		name, _ := entity.Path("name").Data().(string)
		roleAttributes := entity.Path("roleAttributes").String()
		if _, err := fmt.Fprintf(o.Out, "id: %v    name: %v    role attributes: %v\n", id, name, roleAttributes); err != nil {
			return err
		}
	}
	return nil
}

func runListEdgeRouterPolicies(o *commonOptions) error {
	children, err := listEntitiesWithOptions("edge-router-policies", o)
	if err != nil {
		return err
	}
	return outputEdgeRouterPolicies(o, children)
}

func outputEdgeRouterPolicies(o *commonOptions, children []*gabs.Container) error {
	if o.OutputJSONResponse {
		return nil
	}

	for _, entity := range children {
		id, _ := entity.Path("id").Data().(string)
		name, _ := entity.Path("name").Data().(string)
		edgeRouterRoles := entity.Path("edgeRouterRoles").String()
		identityRoles := entity.Path("identityRoles").String()
		_, err := fmt.Fprintf(o.Out, "id: %v    name: %v    edge router roles: %v    identity roles: %v\n", id, name, edgeRouterRoles, identityRoles)
		if err != nil {
			return err
		}
	}
	return nil
}

func runListTerminators(o *commonOptions) error {
	children, err := listEntitiesWithOptions("terminators", o)
	if err != nil {
		return err
	}
	return outputTerminators(o, children)
}

func outputTerminators(o *commonOptions, children []*gabs.Container) error {
	if o.OutputJSONResponse {
		return nil
	}

	for _, entity := range children {
		id, _ := entity.Path("id").Data().(string)
		serviceId := entity.Path("serviceId").Data().(string)
		routerId := entity.Path("routerId").Data().(string)
		binding := entity.Path("binding").Data().(string)
		address := entity.Path("address").Data().(string)
		_, err := fmt.Fprintf(o.Out, "id: %v    serviceId: %v    routerId: %v    binding: %v    address: %v\n",
			id, serviceId, routerId, binding, address)
		if err != nil {
			return err
		}
	}
	return nil
}

func runListServices(asIdentity string, configTypes string, roleFilters []string, roleSemantic string, options *commonOptions) error {
	params := url.Values{}
	if len(options.Args) > 0 {
		params.Add("filter", options.Args[0])
	}
	if asIdentity != "" {
		params.Add("asIdentity", asIdentity)
	}
	if configTypes != "" {
		params.Add("configTypes", configTypes)
	}
	for _, roleFilter := range roleFilters {
		params.Add("roleFilter", roleFilter)
	}
	if roleSemantic != "" {
		params.Add("roleSemantic", roleSemantic)
	}
	children, err := listEntitiesOfType("services", params, options.OutputJSONResponse, options.Out)
	if err != nil {
		return err
	}
	return outputServices(options, children)
}

func outputServices(o *commonOptions, children []*gabs.Container) error {
	if o.OutputJSONResponse {
		return nil
	}

	for _, entity := range children {
		id, _ := entity.Path("id").Data().(string)
		name, _ := entity.Path("name").Data().(string)
		terminatorStrategy, _ := entity.Path("terminatorStrategy").Data().(string)
		roleAttributes := entity.Path("roleAttributes").String()
		_, err := fmt.Fprintf(o.Out, "id: %v    name: %v    terminator strategy: %v    role attributes: %v\n", id, name, terminatorStrategy, roleAttributes)
		if err != nil {
			return err
		}
	}

	return nil
}

func runListServiceEdgeRouterPolices(o *commonOptions) error {
	children, err := listEntitiesWithOptions("service-edge-router-policies", o)
	if err != nil {
		return err
	}
	return outputServiceEdgeRouterPolicies(o, children)
}

func outputServiceEdgeRouterPolicies(o *commonOptions, children []*gabs.Container) error {
	if o.OutputJSONResponse {
		return nil
	}

	for _, entity := range children {
		id, _ := entity.Path("id").Data().(string)
		name, _ := entity.Path("name").Data().(string)
		edgeRouterRoles := entity.Path("edgeRouterRoles").String()
		serviceRoles := entity.Path("serviceRoles").String()
		_, err := fmt.Fprintf(o.Out, "id: %v    name: %v    edge router roles: %v    service roles: %v\n", id, name, edgeRouterRoles, serviceRoles)
		if err != nil {
			return err
		}
	}
	return nil
}

func runListServicePolices(o *commonOptions) error {
	children, err := listEntitiesWithOptions("service-policies", o)
	if err != nil {
		return err
	}
	return outputServicePolicies(o, children)
}

func outputServicePolicies(o *commonOptions, children []*gabs.Container) error {
	if o.OutputJSONResponse {
		return nil
	}

	for _, entity := range children {
		id, _ := entity.Path("id").Data().(string)
		name, _ := entity.Path("name").Data().(string)
		policyType, _ := entity.Path("type").Data().(string)
		identityRoles := entity.Path("identityRoles").String()
		serviceRoles := entity.Path("serviceRoles").String()
		_, err := fmt.Fprintf(o.Out, "id: %v    name: %v    type: %v    service roles: %v    identity roles: %v\n", id, name, policyType, serviceRoles, identityRoles)
		if err != nil {
			return err
		}
	}
	return nil
}

// runListIdentities implements the command to list identities
func runListIdentities(roleFilters []string, roleSemantic string, options *commonOptions) error {
	params := url.Values{}
	if len(options.Args) > 0 {
		params.Add("filter", options.Args[0])
	}
	for _, roleFilter := range roleFilters {
		params.Add("roleFilter", roleFilter)
	}
	if roleSemantic != "" {
		params.Add("roleSemantic", roleSemantic)
	}
	children, err := listEntitiesOfType("identities", params, options.OutputJSONResponse, options.Out)
	if err != nil {
		return err
	}
	return outputIdentities(options, children)
}

// outputIdentities implements the command to list identities
func outputIdentities(o *commonOptions, children []*gabs.Container) error {
	if o.OutputJSONResponse {
		return nil
	}

	for _, entity := range children {
		id, _ := entity.Path("id").Data().(string)
		name, _ := entity.Path("name").Data().(string)
		typeName, _ := entity.Path("type.name").Data().(string)
		roleAttributes := entity.Path("roleAttributes").String()
		if _, err := fmt.Fprintf(o.Out, "id: %v    name: %v    type: %v    role attributes: %v\n", id, name, typeName, roleAttributes); err != nil {
			return err
		}
	}

	return nil
}

func runListCAs(o *commonOptions) error {
	children, err := listEntitiesWithOptions("cas", o)
	if err != nil {
		return err
	}

	if o.OutputJSONResponse {
		return nil
	}

	for _, entity := range children {
		id, _ := entity.Path("id").Data().(string)
		name, _ := entity.Path("name").Data().(string)
		cluster, _ := entity.Path("cluster.id").Data().(string)
		if _, err := fmt.Fprintf(o.Out, "id: %v    name: %v    cluster-id: %v\n", id, name, cluster); err != nil {
			return err
		}
	}

	return nil
}

func runListConfigTypes(o *commonOptions) error {
	children, err := listEntitiesWithOptions("config-types", o)
	if err != nil {
		return err
	}

	if o.OutputJSONResponse {
		return nil
	}

	for _, entity := range children {
		id, _ := entity.Path("id").Data().(string)
		name, _ := entity.Path("name").Data().(string)
		if _, err := fmt.Fprintf(o.Out, "id:   %v    name: %v\n", id, name); err != nil {
			return err
		}
	}

	return nil
}

func runListConfigs(o *commonOptions) error {
	children, err := listEntitiesWithOptions("configs", o)
	if err != nil {
		return err
	}
	return outputConfigs(o, children)
}

func outputConfigs(o *commonOptions, children []*gabs.Container) error {

	if o.OutputJSONResponse {
		return nil
	}

	for _, entity := range children {
		id, _ := entity.Path("id").Data().(string)
		name, _ := entity.Path("name").Data().(string)
		configType, _ := entity.Path("type").Data().(string)
		data, _ := entity.Path("data").Data().(map[string]interface{})
		formattedData, err := json.MarshalIndent(data, "      ", "    ")
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(o.Out, "id:   %v\nname: %v\ntype: %v\ndata: %v\n\n", id, name, configType, string(formattedData)); err != nil {
			return err
		}
	}

	return nil
}

func runListApiSessions(o *commonOptions) error {
	children, err := listEntitiesWithOptions("api-sessions", o)
	if err != nil {
		return err
	}

	if o.OutputJSONResponse {
		return nil
	}

	for _, entity := range children {
		id, _ := entity.Path("id").Data().(string)
		sessionToken, _ := entity.Path("token").Data().(string)
		identityName, _ := entity.Path("identity.name").Data().(string)
		if _, err = fmt.Fprintf(o.Out, "id: %v    token: %v    identity: %v\n", id, sessionToken, identityName); err != nil {
			return err
		}
	}

	return err
}

func runListSessions(o *commonOptions) error {
	children, err := listEntitiesWithOptions("sessions", o)

	if err != nil {
		return err
	}

	if o.OutputJSONResponse {
		return nil
	}

	for _, entity := range children {
		id, _ := entity.Path("id").Data().(string)
		sessionId, _ := entity.Path("apiSession.id").Data().(string)
		serviceName, _ := entity.Path("service.name").Data().(string)
		sessionType, _ := entity.Path("type").Data().(string)
		if _, err := fmt.Fprintf(o.Out, "id: %v    sessionId: %v    serviceName: %v     type: %v\n", id, sessionId, serviceName, sessionType); err != nil {
			return err
		}
	}

	return err
}

func runListEdgeRouterRoleAttributes(o *commonOptions) error {
	return runListRoleAttributes("edge-router", o)
}

func runListIdentityRoleAttributes(o *commonOptions) error {
	return runListRoleAttributes("identity", o)
}

func runListServiceRoleAttributes(o *commonOptions) error {
	return runListRoleAttributes("service", o)
}

func runListRoleAttributes(entityType string, o *commonOptions) error {
	children, err := listEntitiesWithOptions(entityType+"-role-attributes", o)

	if err != nil {
		return err
	}

	if o.OutputJSONResponse {
		return nil
	}

	for _, entity := range children {
		if _, err := fmt.Fprintf(o.Out, "role-attribute: %v\n", entity.Data().(string)); err != nil {
			return err
		}
	}

	return err
}

func runListChilden(parentType, childType string, o *commonOptions, outputF outputFunction) error {
	idOrName := o.Args[0]
	parentId, err := mapNameToID(parentType, idOrName)
	if err != nil {
		return err
	}

	filter := ""
	if len(o.Args) > 1 {
		filter = o.Args[1]
	}

	children, err := filterSubEntitiesOfType(parentType, childType, parentId, filter, o)
	if err != nil {
		return err
	}

	if o.OutputJSONResponse {
		return nil
	}

	return outputF(o, children)
}
