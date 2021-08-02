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
	"github.com/Jeffail/gabs"
	"github.com/openziti/edge/rest_management_api_client/certificate_authority"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/foundation/util/errorz"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/common"
	cmdutil "github.com/openziti/ziti/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/ziti/util"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"io"
	"net/url"
	"reflect"
	"strings"
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

	newOptions := func() *edgeOptions {
		return &edgeOptions{
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
	cmd.AddCommand(newListCmdForEntityType("transit-routers", runListTransitRouters, newOptions()))

	cmd.AddCommand(newListCmdForEntityType("edge-router-role-attributes", runListEdgeRouterRoleAttributes, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("identity-role-attributes", runListIdentityRoleAttributes, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("service-role-attributes", runListServiceRoleAttributes, newOptions()))

	cmd.AddCommand(newListCmdForEntityType("posture-checks", runListPostureChecks, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("posture-check-types", runListPostureCheckTypes, newOptions()))

	configTypeListRootCmd := newEntityListRootCmd("config-type")
	configTypeListRootCmd.AddCommand(newSubListCmdForEntityType("config-type", "configs", outputConfigs, newOptions()))

	edgeRouterListRootCmd := newEntityListRootCmd("edge-router")
	edgeRouterListRootCmd.AddCommand(newSubListCmdForEntityType("edge-routers", "edge-router-policies", outputEdgeRouterPolicies, newOptions()))
	edgeRouterListRootCmd.AddCommand(newSubListCmdForEntityType("edge-routers", "service-edge-router-policies", outputServiceEdgeRouterPolicies, newOptions()))
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
	identityListRootCmd.AddCommand(newSubListCmdForEntityType("identities", "service-configs", outputServiceConfigs, newOptions()))

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
	servicePolicyListRootCmd.AddCommand(newSubListCmdForEntityType("service-policies", "posture-checks", outputPostureChecks, newOptions()))

	cmd.AddCommand(configTypeListRootCmd,
		edgeRouterListRootCmd,
		edgeRouterPolicyListRootCmd,
		identityListRootCmd,
		serviceEdgeRouterPolicyListRootCmd,
		serviceListRootCmd,
		servicePolicyListRootCmd,
	)

	return cmd
}

type paging struct {
	limit  int64
	offset int64
	count  int64
	errorz.ErrorHolderImpl
}

func newPagingInfo(meta *rest_model.Meta) paging {
	if meta != nil && meta.Pagination != nil {
		pagingInfo := paging{
			limit:  *meta.Pagination.Limit,
			offset: *meta.Pagination.Offset,
			count:  *meta.Pagination.TotalCount,
		}

		return pagingInfo
	}

	return paging{}
}

func (p *paging) output(o *edgeOptions) {
	if p.HasError() {
		_, _ = fmt.Fprintf(o.Out, "unable to retrieve paging information: %v\n", p.Err)
	} else if p.count == 0 {
		_, _ = fmt.Fprintln(o.Out, "results: none")
	} else {
		first := p.offset + 1
		last := p.offset + p.limit
		if last > p.count || last < 0 { // if p.limit is maxint, last will rollover and be negative
			last = p.count
		}
		_, _ = fmt.Fprintf(o.Out, "results: %v-%v of %v\n", first, last, p.count)
	}
}

type listCommandRunner func(*edgeOptions) error

type outputFunction func(o *edgeOptions, children []*gabs.Container, pagingInfo *paging) error

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
func newListCmdForEntityType(entityType string, command listCommandRunner, options *edgeOptions) *cobra.Command {
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
	options.AddCommonFlags(cmd)

	return cmd
}

// newListServicesCmd creates the list command for the given entity type
func newListServicesCmd(options *edgeOptions) *cobra.Command {
	var asIdentity string
	var configTypes []string
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
	cmd.Flags().StringVar(&asIdentity, "as-identity", "", "Allow admins to see services as they would be seen by a different identity")
	cmd.Flags().StringSliceVar(&configTypes, "config-types", nil, "Override which config types to view on services")
	cmd.Flags().StringSliceVar(&roleFilters, "role-filters", nil, "Allow filtering by roles")
	cmd.Flags().StringVar(&roleSemantic, "role-semantic", "", "Specify which roles semantic to use ")
	options.AddCommonFlags(cmd)

	return cmd
}

// newListEdgeRoutersCmd creates the list command for the given entity type
func newListEdgeRoutersCmd(options *edgeOptions) *cobra.Command {
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
	cmd.Flags().StringSliceVar(&roleFilters, "role-filters", nil, "Allow filtering by roles")
	cmd.Flags().StringVar(&roleSemantic, "role-semantic", "", "Specify which roles semantic to use ")
	options.AddCommonFlags(cmd)

	return cmd
}

// newListEdgeRoutersCmd creates the list command for the given entity type
func newListIdentitiesCmd(options *edgeOptions) *cobra.Command {
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
	cmd.Flags().StringSliceVar(&roleFilters, "role-filters", nil, "Allow filtering by roles")
	cmd.Flags().StringVar(&roleSemantic, "role-semantic", "", "Specify which roles semantic to use ")
	options.AddCommonFlags(cmd)

	return cmd
}

// newSubListCmdForEntityType creates the list command for the given entity type
func newSubListCmdForEntityType(entityType string, subType string, outputF outputFunction, options *edgeOptions) *cobra.Command {
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
	options.AddCommonFlags(cmd)

	return cmd
}

// listEntitiesOfType queries the Ziti Controller for entities of the given type
func listEntitiesWithOptions(entityType string, options *edgeOptions) ([]*gabs.Container, *paging, error) {
	params := url.Values{}
	if len(options.Args) > 0 {
		params.Add("filter", options.Args[0])
	}

	return listEntitiesOfType(entityType, params, options.OutputJSONResponse, options.Out, options.Timeout, options.Verbose)
}

func filterEntitiesOfType(entityType string, filter string, logJSON bool, out io.Writer, timeout int, verbose bool) ([]*gabs.Container, *paging, error) {
	params := url.Values{}
	params.Add("filter", filter)
	return listEntitiesOfType(entityType, params, logJSON, out, timeout, verbose)
}

// listEntitiesOfType queries the Ziti Controller for entities of the given type
func listEntitiesOfType(entityType string, params url.Values, logJSON bool, out io.Writer, timeout int, verbose bool) ([]*gabs.Container, *paging, error) {
	jsonParsed, err := util.EdgeControllerList(entityType, params, logJSON, out, timeout, verbose)

	if err != nil {
		return nil, nil, err
	}

	children, err := jsonParsed.S("data").Children()
	return children, getPaging(jsonParsed), err
}

func toInt64(c *gabs.Container, path string, errorHolder errorz.ErrorHolder) int64 {
	data := c.S(path).Data()
	if data == nil {
		errorHolder.SetError(errors.Errorf("%v not found", path))
		return 0
	}
	val, ok := data.(float64)
	if !ok {
		errorHolder.SetError(errors.Errorf("%v not a number, it's a %v", path, reflect.TypeOf(data)))
		return 0
	}
	return int64(val)
}

func getPaging(c *gabs.Container) *paging {
	pagingInfo := &paging{}
	pagination := c.S("meta", "pagination")
	if pagination != nil {
		pagingInfo.limit = toInt64(pagination, "limit", pagingInfo)
		pagingInfo.offset = toInt64(pagination, "offset", pagingInfo)
		pagingInfo.count = toInt64(pagination, "totalCount", pagingInfo)
	} else {
		pagingInfo.SetError(errors.New("meta.pagination section not found in result"))
	}
	return pagingInfo
}

// listEntitiesOfType queries the Ziti Controller for entities of the given type
func filterSubEntitiesOfType(entityType, subType, entityId, filter string, o *edgeOptions) ([]*gabs.Container, *paging, error) {
	jsonParsed, err := util.EdgeControllerListSubEntities(entityType, subType, entityId, filter, o.OutputJSONResponse, o.Out, o.Timeout, o.Verbose)

	if err != nil {
		return nil, nil, err
	}

	children, err := jsonParsed.S("data").Children()
	if err == gabs.ErrNotObjOrArray {
		return nil, getPaging(jsonParsed), nil
	}
	return children, getPaging(jsonParsed), err
}

func runListEdgeRouters(roleFilters []string, roleSemantic string, options *edgeOptions) error {
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
	children, paging, err := listEntitiesOfType("edge-routers", params, options.OutputJSONResponse, options.Out, options.Timeout, options.Verbose)
	if err != nil {
		return err
	}

	return outputEdgeRouters(options, children, paging)
}

func outputEdgeRouters(o *edgeOptions, children []*gabs.Container, pagingInfo *paging) error {
	if o.OutputJSONResponse {
		return nil
	}

	for _, entity := range children {
		id, _ := entity.Path("id").Data().(string)
		name, _ := entity.Path("name").Data().(string)
		isOnline, _ := entity.Path("isOnline").Data().(bool)
		roleAttributes := entity.Path("roleAttributes").String()
		if _, err := fmt.Fprintf(o.Out, "id: %v    name: %v    isOnline: %v    role attributes: %v\n", id, name, isOnline, roleAttributes); err != nil {
			return err
		}
	}
	pagingInfo.output(o)
	return nil
}

func runListEdgeRouterPolicies(o *edgeOptions) error {
	children, paging, err := listEntitiesWithOptions("edge-router-policies", o)
	if err != nil {
		return err
	}
	return outputEdgeRouterPolicies(o, children, paging)
}

func outputEdgeRouterPolicies(o *edgeOptions, children []*gabs.Container, pagingInfo *paging) error {
	if o.OutputJSONResponse {
		return nil
	}

	for _, entity := range children {
		id, _ := entity.Path("id").Data().(string)
		name, _ := entity.Path("name").Data().(string)

		identityRoles, err := mapRoleIdsToNames(entity, "identityRoles", "identities", *o)
		if err != nil {
			return err
		}

		edgeRouterRoles, err := mapRoleIdsToNames(entity, "edgeRouterRoles", "edge-routers", *o)
		if err != nil {
			return err
		}

		_, err = fmt.Fprintf(o.Out, "id: %v    name: %v    edge router roles: %v    identity roles: %v\n", id, name, edgeRouterRoles, identityRoles)
		if err != nil {
			return err
		}
	}
	pagingInfo.output(o)
	return nil
}

func runListTerminators(o *edgeOptions) error {
	children, pagingInfo, err := listEntitiesWithOptions("terminators", o)
	if err != nil {
		return err
	}
	return outputTerminators(o, children, pagingInfo)
}

func outputTerminators(o *edgeOptions, children []*gabs.Container, pagingInfo *paging) error {
	if o.OutputJSONResponse {
		return nil
	}

	for _, entity := range children {
		id, _ := entity.Path("id").Data().(string)
		service := entity.Path("service.name").Data().(string)
		router := entity.Path("router.id").Data().(string)
		binding := entity.Path("binding").Data().(string)
		address := entity.Path("address").Data().(string)
		identity := entity.Path("identity").Data().(string)
		staticCost := entity.Path("cost").Data().(float64)
		precedence := entity.Path("precedence").Data().(string)
		dynamicCost := entity.Path("dynamicCost").Data().(float64)
		_, err := fmt.Fprintf(o.Out, "id: %v    service: %v    router: %v    binding: %v    address: %v    identity: %v    cost: %v    precedence: %v    dynamic-cost: %v\n",
			id, service, router, binding, address, identity, staticCost, precedence, dynamicCost)
		if err != nil {
			return err
		}
	}
	pagingInfo.output(o)
	return nil
}

func runListServices(asIdentity string, configTypes []string, roleFilters []string, roleSemantic string, options *edgeOptions) error {
	params := url.Values{}
	if len(options.Args) > 0 {
		params.Add("filter", options.Args[0])
	}
	if asIdentity != "" {
		params.Add("asIdentity", asIdentity)
	}

	if len(configTypes) == 1 && strings.EqualFold("all", configTypes[0]) {
		params.Add("configTypes", "all")
	} else {
		if configTypes, err := mapNamesToIDs("config-types", *options, configTypes...); err != nil {
			return err
		} else {
			for _, configType := range configTypes {
				params.Add("configTypes", configType)
			}
		}
	}
	for _, roleFilter := range roleFilters {
		params.Add("roleFilter", roleFilter)
	}
	if roleSemantic != "" {
		params.Add("roleSemantic", roleSemantic)
	}
	children, pagingInfo, err := listEntitiesOfType("services", params, options.OutputJSONResponse, options.Out, options.Timeout, options.Verbose)
	if err != nil {
		return err
	}
	return outputServices(options, children, pagingInfo)
}

func outputServices(o *edgeOptions, children []*gabs.Container, pagingInfo *paging) error {
	if o.OutputJSONResponse {
		return nil
	}

	for _, entity := range children {
		id, _ := entity.Path("id").Data().(string)
		name, _ := entity.Path("name").Data().(string)
		encryptionRequired, _ := entity.Path("encryptionRequired").Data().(bool)
		terminatorStrategy, _ := entity.Path("terminatorStrategy").Data().(string)
		roleAttributes := entity.Path("roleAttributes").String()

		_, err := fmt.Fprintf(o.Out, "id: %v    name: %v    encryption required: %v    terminator strategy: %v    role attributes: %v\n", id, name, encryptionRequired, terminatorStrategy, roleAttributes)
		if err != nil {
			return err
		}
	}
	pagingInfo.output(o)
	return nil
}

func outputServiceConfigs(o *edgeOptions, children []*gabs.Container, pagingInfo *paging) error {
	if o.OutputJSONResponse {
		return nil
	}

	for _, entity := range children {
		service, _ := entity.Path("service").Data().(string)
		serviceName, _ := mapIdToName("services", service, *o)
		config, _ := entity.Path("config").Data().(string)
		configName, _ := mapIdToName("configs", config, *o)
		_, err := fmt.Fprintf(o.Out, "service: %v    config: %v\n", serviceName, configName)
		if err != nil {
			return err
		}
	}
	pagingInfo.output(o)
	return nil
}

func runListServiceEdgeRouterPolices(o *edgeOptions) error {
	children, pagingInfo, err := listEntitiesWithOptions("service-edge-router-policies", o)
	if err != nil {
		return err
	}
	return outputServiceEdgeRouterPolicies(o, children, pagingInfo)
}

func outputServiceEdgeRouterPolicies(o *edgeOptions, children []*gabs.Container, pagingInfo *paging) error {
	if o.OutputJSONResponse {
		return nil
	}

	for _, entity := range children {
		id, _ := entity.Path("id").Data().(string)
		name, _ := entity.Path("name").Data().(string)
		edgeRouterRoles, err := mapRoleIdsToNames(entity, "edgeRouterRoles", "edge-routers", *o)
		if err != nil {
			return err
		}
		serviceRoles, err := mapRoleIdsToNames(entity, "serviceRoles", "services", *o)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(o.Out, "id: %v    name: %v    edge router roles: %v    service roles: %v\n", id, name, edgeRouterRoles, serviceRoles)
		if err != nil {
			return err
		}
	}
	pagingInfo.output(o)
	return nil
}

func runListServicePolices(o *edgeOptions) error {
	children, pagingInfo, err := listEntitiesWithOptions("service-policies", o)
	if err != nil {
		return err
	}
	return outputServicePolicies(o, children, pagingInfo)
}

func outputServicePolicies(o *edgeOptions, children []*gabs.Container, pagingInfo *paging) error {
	if o.OutputJSONResponse {
		return nil
	}

	for _, entity := range children {
		id, _ := entity.Path("id").Data().(string)
		name, _ := entity.Path("name").Data().(string)
		policyType, _ := entity.Path("type").Data().(string)

		identityRoles, err := mapRoleIdsToNames(entity, "identityRoles", "identities", *o)
		if err != nil {
			return err
		}

		serviceRoles, err := mapRoleIdsToNames(entity, "serviceRoles", "services", *o)
		if err != nil {
			return err
		}
		postureCheckRoles, err := mapRoleIdsToNames(entity, "postureCheckRoles", "posture-checks", *o)
		if err != nil {
			return err
		}

		_, err = fmt.Fprintf(o.Out, "id: %v    name: %v    type: %v    service roles: %v    identity roles: %v posture check roles: %v\n", id, name, policyType, serviceRoles, identityRoles, postureCheckRoles)
		if err != nil {
			return err
		}
	}
	pagingInfo.output(o)
	return nil
}

func mapRoleIdsToNames(c *gabs.Container, path string, entityType string, o edgeOptions) ([]string, error) {
	jsonValues := c.Path(path).Data()
	if jsonValues == nil {
		return nil, nil
	}

	values := jsonValues.([]interface{})

	var result []string
	for _, val := range values {
		str := val.(string)
		if strings.HasPrefix(str, "@") {
			id := strings.TrimPrefix(str, "@")
			name, err := mapIdToName(entityType, id, o)
			if err != nil {
				return nil, err
			}
			result = append(result, "@"+name)
		} else {
			result = append(result, str)
		}
	}
	return result, nil
}

// runListIdentities implements the command to list identities
func runListIdentities(roleFilters []string, roleSemantic string, options *edgeOptions) error {
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
	children, pagingInfo, err := listEntitiesOfType("identities", params, options.OutputJSONResponse, options.Out, options.Timeout, options.Verbose)
	if err != nil {
		return err
	}
	return outputIdentities(options, children, pagingInfo)
}

// outputIdentities implements the command to list identities
func outputIdentities(o *edgeOptions, children []*gabs.Container, pagingInfo *paging) error {
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
	pagingInfo.output(o)

	return nil
}

func outputPostureCheck(o *edgeOptions, entity *gabs.Container) error {
	id, _ := entity.Path("id").Data().(string)
	typeId, _ := entity.Path("typeId").Data().(string)
	name, _ := entity.Path("name").Data().(string)
	roleAttributes := entity.Path("roleAttributes").String()

	config := ""

	switch typeId {
	case "MFA":
		timeoutFloat, _ := entity.Path("timeoutSeconds").Data().(float64)
		timeout := int64(timeoutFloat)
		promptOnWake, _ := entity.Path("promptOnWake").Data().(bool)
		promptOnUnlock, _ := entity.Path("promptOnUnlock").Data().(bool)
		ignoreLegacyEndpoints, _ := entity.Path("ignoreLegacyEndpoints").Data().(bool)
		config = fmt.Sprintf("timeout: %d, wake: %t, unlock: %t, ignore: %t", timeout, promptOnWake, promptOnUnlock, ignoreLegacyEndpoints)
	case "MAC":
		containers, _ := entity.Path("macAddresses").Children()
		config = containerArrayToString(containers, 4)
	case "DOMAIN":
		containers, _ := entity.Path("domains").Children()
		config = containerArrayToString(containers, 4)
	case "OS":
		operatingSystems, _ := entity.Path("operatingSystems").Children()
		config = strings.Join(postureCheckOsToStrings(operatingSystems), ",")
	case "PROCESS_MULTI":
		postureCheck := rest_model.PostureCheckProcessMultiDetail{}
		if err := postureCheck.UnmarshalJSON(entity.Bytes()); err != nil {
			return err
		}

		baseConfig := fmt.Sprintf("(SEMANTIC: %s)", *postureCheck.Semantic)

		if _, err := fmt.Fprintf(o.Out, "id: %-10v    type: %-10v    name: %-15v    role attributes: %-10s     param: %v\n", id, typeId, name, roleAttributes, baseConfig); err != nil {
			return err
		}

		for _, process := range postureCheck.Processes {
			process.SignerFingerprints = getEllipsesStrings(process.SignerFingerprints, 4, 2)
			process.Hashes = getEllipsesStrings(process.Hashes, 4, 2)
			_, _ = fmt.Fprintf(o.Out, "\t(OS: %s, PATH: %s, HASHES: %s, SIGNER: %s)\n", *process.OsType, *process.Path, strings.Join(process.Hashes, ","), strings.Join(process.SignerFingerprints, ", "))
		}

		return nil

	case "PROCESS":
		process := entity.Path("process")

		os := process.Path("osType").Data().(string)
		path := process.Path("path").Data().(string)

		var hashStrings []string
		if val := process.Path("hashes").Data(); val != nil {
			hashes := val.([]interface{})

			for _, hash := range hashes {
				hashStr := hash.(string)
				hashStrings = append(hashStrings, getEllipsesString(hashStr, 4, 2))
			}
		}
		signerFingerprint := "N/A"
		if val := process.Path("signerFingerprint").Data(); val != nil {
			if valStr := val.(string); valStr != "" {
				signerFingerprint = getEllipsesString(valStr, 4, 2)
			}
		}

		if len(hashStrings) == 0 {
			hashStrings = append(hashStrings, "N/A")
		}

		config = fmt.Sprintf("\n\t(OS: %s, PATH: %s, HASHES: %s, SIGNER: %s)", os, path, strings.Join(hashStrings, ","), signerFingerprint)
	}

	if _, err := fmt.Fprintf(o.Out, "id: %-10v    type: %-10v    name: %-15v    role attributes: %-10s     param: %v\n", id, typeId, name, roleAttributes, config); err != nil {
		return err
	}

	return nil
}

func getEllipsesString(val string, lead, lag int) string {
	total := lead + lag + 3

	if len(val) <= total {
		return val
	}

	return val[0:lead] + "..." + val[len(val)-lag:]
}

func getEllipsesStrings(values []string, lead, lag int) []string {
	var ret []string
	for _, val := range values {
		ret = append(ret, getEllipsesString(val, lead, lag))
	}

	return ret
}

func outputPostureChecks(o *edgeOptions, children []*gabs.Container, pagingInfo *paging) error {
	if o.OutputJSONResponse {
		return nil
	}

	for _, entity := range children {
		if err := outputPostureCheck(o, entity); err != nil {
			return err
		}
	}
	pagingInfo.output(o)

	return nil
}

func runListCAs(o *edgeOptions) error {
	client, err := util.NewEdgeManagementClient(o)

	if err != nil {
		return err
	}

	var filter *string = nil

	if len(o.Args) > 0 {
		filter = &o.Args[0]
	}

	context, cancelContext := o.TimeoutContext()
	defer cancelContext()

	result, err := client.CertificateAuthority.ListCas(&certificate_authority.ListCasParams{
		Filter:  filter,
		Context: context,
	}, nil)

	if err != nil {
		return util.WrapIfApiError(err)
	}

	if o.OutputJSONResponse {
		return nil
	}

	payload := result.GetPayload()

	if payload == nil {
		return errors.New("unexpected empty response payload")
	}

	for _, entity := range result.GetPayload().Data {
		id := *entity.ID
		name := *entity.Name
		identityRoles := make([]string, 0)

		for _, role := range entity.IdentityRoles {
			identityRoles = append(identityRoles, role)
		}

		isVerified := *entity.IsVerified
		fingerprint := ""
		token := ""
		if isVerified {
			fingerprint = *entity.Fingerprint
		} else {
			token = entity.VerificationToken.String()
		}

		identityNameFormat := *entity.IdentityNameFormat

		flags := ""

		if isVerified {
			flags = flags + "V"
		}

		if *entity.IsAutoCaEnrollmentEnabled {
			flags = flags + "A"
		}

		if *entity.IsOttCaEnrollmentEnabled {
			flags = flags + "O"
		}

		if *entity.IsAuthEnabled {
			flags = flags + "E"
		}

		flags = "[" + flags + "]"

		if _, err := fmt.Fprintf(o.Out, "id: %v    name: %v   token: %v    identityRoles: %v    flags: %s    identityNameFormat: %v    fingerprint: %v\n",
			id, name, token, identityRoles, flags, identityNameFormat, getEllipsesString(fingerprint, 4, 2),
		); err != nil {
			return err
		}
	}

	pagingInfo := newPagingInfo(payload.Meta)
	pagingInfo.output(o)

	_, _ = fmt.Fprint(o.Out, "\nFlags: (V) Verified, (A) AutoCa Enrollment, (O) OttCA Enrollment, (E) Authentication Enabled\n\n")

	return nil
}

func runListConfigTypes(o *edgeOptions) error {
	children, pagingInfo, err := listEntitiesWithOptions("config-types", o)
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
	pagingInfo.output(o)
	return nil
}

func runListConfigs(o *edgeOptions) error {
	children, pagingInfo, err := listEntitiesWithOptions("configs", o)
	if err != nil {
		return err
	}
	return outputConfigs(o, children, pagingInfo)
}

func outputConfigs(o *edgeOptions, children []*gabs.Container, pagingInfo *paging) error {
	if o.OutputJSONResponse {
		return nil
	}

	for _, entity := range children {
		id, _ := entity.Path("id").Data().(string)
		name, _ := entity.Path("name").Data().(string)
		configType, _ := entity.Path("configType.name").Data().(string)
		data, _ := entity.Path("data").Data().(map[string]interface{})
		formattedData, err := json.MarshalIndent(data, "      ", "    ")
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(o.Out, "id:   %v\nname: %v\ntype: %v\ndata: %v\n\n", id, name, configType, string(formattedData)); err != nil {
			return err
		}
	}
	pagingInfo.output(o)
	return nil
}

func runListApiSessions(o *edgeOptions) error {
	children, pagingInfo, err := listEntitiesWithOptions("api-sessions", o)
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
	pagingInfo.output(o)
	return err
}

func runListSessions(o *edgeOptions) error {
	children, pagingInfo, err := listEntitiesWithOptions("sessions", o)

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
	pagingInfo.output(o)
	return err
}

func runListTransitRouters(o *edgeOptions) error {
	children, pagingInfo, err := listEntitiesWithOptions("transit-routers", o)

	if err != nil {
		return err
	}

	if o.OutputJSONResponse {
		return nil
	}

	for _, entity := range children {
		id, _ := entity.Path("id").Data().(string)
		name, _ := entity.Path("name").Data().(string)
		if _, err := fmt.Fprintf(o.Out, "id: %v    name: %v\n", id, name); err != nil {
			return err
		}
	}
	pagingInfo.output(o)
	return err
}

func runListEdgeRouterRoleAttributes(o *edgeOptions) error {
	return runListRoleAttributes("edge-router", o)
}

func runListIdentityRoleAttributes(o *edgeOptions) error {
	return runListRoleAttributes("identity", o)
}

func runListServiceRoleAttributes(o *edgeOptions) error {
	return runListRoleAttributes("service", o)
}

func runListRoleAttributes(entityType string, o *edgeOptions) error {
	children, pagingInfo, err := listEntitiesWithOptions(entityType+"-role-attributes", o)

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
	pagingInfo.output(o)
	return err
}

func runListChilden(parentType, childType string, o *edgeOptions, outputF outputFunction) error {
	idOrName := o.Args[0]
	parentId, err := mapNameToID(parentType, idOrName, *o)
	if err != nil {
		return err
	}

	filter := ""
	if len(o.Args) > 1 {
		filter = o.Args[1]
	}

	children, pagingInfo, err := filterSubEntitiesOfType(parentType, childType, parentId, filter, o)
	if err != nil {
		return err
	}

	if o.OutputJSONResponse {
		return nil
	}

	return outputF(o, children, pagingInfo)
}

func runListPostureChecks(o *edgeOptions) error {
	children, pagingInfo, err := listEntitiesWithOptions("posture-checks", o)

	if err != nil {
		return err
	}

	if err := outputPostureChecks(o, children, pagingInfo); err != nil {
		return err
	}

	return err
}

func containerArrayToString(containers []*gabs.Container, limit int) string {
	var values []string
	for _, container := range containers {
		value := container.Data().(string)
		values = append(values, value)
	}
	valuesLength := len(values)
	if valuesLength > limit {
		values = values[:limit-1]
		values = append(values, fmt.Sprintf(" and %d more", valuesLength-limit))
	}
	return strings.Join(values, ",")
}

func runListPostureCheckTypes(o *edgeOptions) error {
	children, pagingInfo, err := listEntitiesWithOptions("posture-check-types", o)

	if err != nil {
		return err
	}

	if o.OutputJSONResponse {
		return nil
	}

	for _, entity := range children {
		id, _ := entity.Path("id").Data().(string)
		operatingSystems, _ := entity.Path("operatingSystems").Children()
		osInfo := postureCheckOsToStrings(operatingSystems)

		if _, err := fmt.Fprintf(o.Out, "id: %-8s    os: %s\n", id, strings.Join(osInfo, ",")); err != nil {
			return err
		}
	}
	pagingInfo.output(o)
	return err
}

func postureCheckOsToStrings(osContainers []*gabs.Container) []string {
	var ret []string
	for _, os := range osContainers {
		osType := os.Path("type").Data().(string)
		var osVersions []string
		versionsContainer, _ := os.Path("versions").Children()
		for _, versionContainer := range versionsContainer {
			if version := versionContainer.Data().(string); version != "" {
				osVersions = append(osVersions, version)
			}
		}

		if len(osVersions) == 0 {
			osVersions = append(osVersions, "any")
		}

		ret = append(ret, fmt.Sprintf("%s (%s)", osType, strings.Join(osVersions, ",")))
	}

	return ret
}
