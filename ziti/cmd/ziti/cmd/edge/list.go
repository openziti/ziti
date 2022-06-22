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

package edge

import (
	"fmt"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"io"
	"net/url"
	"reflect"
	"sort"
	"strings"

	"github.com/Jeffail/gabs"
	"github.com/openziti/edge/rest_management_api_client/certificate_authority"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/foundation/util/errorz"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/ziti/util"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// newListCmd creates a command object for the "controller list" command
func newListCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "Lists various entities managed by the Ziti Edge Controller",
		Aliases: []string{"ls"},
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			cmdhelper.CheckErr(err)
		},
	}

	newOptions := func() *api.Options {
		return &api.Options{
			CommonOptions: common.CommonOptions{Out: out, Err: errOut},
		}
	}

	cmd.AddCommand(newListCmdForEntityType("api-sessions", runListApiSessions, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("authenticators", runListAuthenticators, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("cas", runListCAs, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("config-types", runListConfigTypes, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("configs", runListConfigs, newOptions()))
	cmd.AddCommand(newListEdgeRoutersCmd(newOptions()))
	cmd.AddCommand(newListCmdForEntityType("edge-router-policies", runListEdgeRouterPolicies, newOptions(), "erps"))
	cmd.AddCommand(newListCmdForEntityType("enrollments", runListEnrollments, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("terminators", runListTerminators, newOptions()))
	cmd.AddCommand(newListIdentitiesCmd(newOptions()))
	cmd.AddCommand(newListServicesCmd(newOptions()))
	cmd.AddCommand(newListCmdForEntityType("service-edge-router-policies", runListServiceEdgeRouterPolices, newOptions(), "serps"))
	cmd.AddCommand(newListCmdForEntityType("service-policies", runListServicePolices, newOptions(), "sps"))
	cmd.AddCommand(newListCmdForEntityType("sessions", runListSessions, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("transit-routers", runListTransitRouters, newOptions()))

	cmd.AddCommand(newListCmdForEntityType("edge-router-role-attributes", runListEdgeRouterRoleAttributes, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("identity-role-attributes", runListIdentityRoleAttributes, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("service-role-attributes", runListServiceRoleAttributes, newOptions()))

	cmd.AddCommand(newListCmdForEntityType("posture-checks", runListPostureChecks, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("posture-check-types", runListPostureCheckTypes, newOptions()))

	configTypeListRootCmd := newEntityListRootCmd("config-type")
	configTypeListRootCmd.AddCommand(newSubListCmdForEntityType("config-type", "configs", outputConfigs, newOptions()))

	edgeRouterListRootCmd := newEntityListRootCmd("edge-router", "er")
	edgeRouterListRootCmd.AddCommand(newSubListCmdForEntityType("edge-routers", "edge-router-policies", outputEdgeRouterPolicies, newOptions()))
	edgeRouterListRootCmd.AddCommand(newSubListCmdForEntityType("edge-routers", "service-edge-router-policies", outputServiceEdgeRouterPolicies, newOptions()))
	edgeRouterListRootCmd.AddCommand(newSubListCmdForEntityType("edge-routers", "identities", outputIdentities, newOptions()))
	edgeRouterListRootCmd.AddCommand(newSubListCmdForEntityType("edge-routers", "services", outputServices, newOptions()))

	edgeRouterPolicyListRootCmd := newEntityListRootCmd("edge-router-policy", "erp")
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

	serviceEdgeRouterPolicyListRootCmd := newEntityListRootCmd("service-edge-router-policy", "serp")
	serviceEdgeRouterPolicyListRootCmd.AddCommand(newSubListCmdForEntityType("service-edge-router-policies", "services", outputServices, newOptions()))
	serviceEdgeRouterPolicyListRootCmd.AddCommand(newSubListCmdForEntityType("service-edge-router-policies", "edge-routers", outputEdgeRouters, newOptions()))

	servicePolicyListRootCmd := newEntityListRootCmd("service-policy", "sp")
	servicePolicyListRootCmd.AddCommand(newSubListCmdForEntityType("service-policies", "services", outputServices, newOptions()))
	servicePolicyListRootCmd.AddCommand(newSubListCmdForEntityType("service-policies", "identities", outputIdentities, newOptions()))
	servicePolicyListRootCmd.AddCommand(newSubListCmdForEntityType("service-policies", "posture-checks", outputPostureChecks, newOptions()))

	cmd.AddCommand(newListCmdForEntityType("summary", runListSummary, newOptions()))

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

func newPagingInfo(meta *rest_model.Meta) *api.Paging {
	if meta != nil && meta.Pagination != nil {
		pagingInfo := &api.Paging{
			Limit:  *meta.Pagination.Limit,
			Offset: *meta.Pagination.Offset,
			Count:  *meta.Pagination.TotalCount,
		}

		return pagingInfo
	}

	return &api.Paging{}
}

type listCommandRunner func(*api.Options) error

type outputFunction func(o *api.Options, children []*gabs.Container, pagingInfo *api.Paging) error

func newEntityListRootCmd(entityType string, aliases ...string) *cobra.Command {
	desc := fmt.Sprintf("list entities related to a %v instance managed by the Ziti Edge Controller", entityType)
	return &cobra.Command{
		Use:     entityType,
		Aliases: aliases,
		Short:   desc,
		Long:    desc,
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			cmdhelper.CheckErr(err)
		},
	}
}

// newListCmdForEntityType creates the list command for the given entity type
func newListCmdForEntityType(entityType string, command listCommandRunner, options *api.Options, aliases ...string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     entityType + " <filter>?",
		Short:   "lists " + entityType + " managed by the Ziti Edge Controller",
		Args:    cobra.MaximumNArgs(1),
		Aliases: aliases,
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
	cmd.Flags().BoolVar(&options.OutputCSV, "csv", false, "Output CSV instead of a formatted table")
	options.AddCommonFlags(cmd)

	return cmd
}

// newListServicesCmd creates the list command for the given entity type
func newListServicesCmd(options *api.Options) *cobra.Command {
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
	cmd.Flags().BoolVar(&options.OutputCSV, "csv", false, "Output CSV instead of a formatted table")
	options.AddCommonFlags(cmd)

	return cmd
}

// newListEdgeRoutersCmd creates the list command for the given entity type
func newListEdgeRoutersCmd(options *api.Options) *cobra.Command {
	var roleFilters []string
	var roleSemantic string

	cmd := &cobra.Command{
		Use:     "edge-routers <filter>?",
		Short:   "lists edge routers managed by the Ziti Edge Controller",
		Long:    "lists edge routers managed by the Ziti Edge Controller",
		Args:    cobra.MaximumNArgs(1),
		Aliases: []string{"ers"},
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
	cmd.Flags().BoolVar(&options.OutputCSV, "csv", false, "Output CSV instead of a formatted table")
	options.AddCommonFlags(cmd)

	return cmd
}

// newListEdgeRoutersCmd creates the list command for the given entity type
func newListIdentitiesCmd(options *api.Options) *cobra.Command {
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
	cmd.Flags().BoolVar(&options.OutputCSV, "csv", false, "Output CSV instead of a formatted table")
	options.AddCommonFlags(cmd)

	return cmd
}

// newSubListCmdForEntityType creates the list command for the given entity type
func newSubListCmdForEntityType(entityType string, subType string, outputF outputFunction, options *api.Options) *cobra.Command {
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
	cmd.Flags().BoolVar(&options.OutputCSV, "csv", false, "Output CSV instead of a formatted table")

	return cmd
}

// ListEntitiesOfType queries the Ziti Controller for entities of the given type
func listEntitiesWithOptions(entityType string, options *api.Options) ([]*gabs.Container, *api.Paging, error) {
	params := url.Values{}
	if len(options.Args) > 0 {
		params.Add("filter", options.Args[0])
	}

	return ListEntitiesOfType(entityType, params, options.OutputJSONResponse, options.Out, options.Timeout, options.Verbose)
}

func ListEntitiesWithFilter(entityType string, filter string) ([]*gabs.Container, *api.Paging, error) {
	params := url.Values{}
	params.Add("filter", filter)
	return ListEntitiesOfType(entityType, params, false, nil, 5, false)
}

func filterEntitiesOfType(entityType string, filter string, logJSON bool, out io.Writer, timeout int, verbose bool) ([]*gabs.Container, *api.Paging, error) {
	params := url.Values{}
	params.Add("filter", filter)
	return ListEntitiesOfType(entityType, params, logJSON, out, timeout, verbose)
}

// ListEntitiesOfType queries the Ziti Controller for entities of the given type
func ListEntitiesOfType(entityType string, params url.Values, logJSON bool, out io.Writer, timeout int, verbose bool) ([]*gabs.Container, *api.Paging, error) {
	jsonParsed, err := util.EdgeControllerList(entityType, params, logJSON, out, timeout, verbose)

	if err != nil {
		return nil, nil, err
	}

	children, err := jsonParsed.S("data").Children()
	return children, api.GetPaging(jsonParsed), err
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

// ListEntitiesOfType queries the Ziti Controller for entities of the given type
func filterSubEntitiesOfType(entityType, subType, entityId, filter string, o *api.Options) ([]*gabs.Container, *api.Paging, error) {
	jsonParsed, err := util.EdgeControllerListSubEntities(entityType, subType, entityId, filter, o.OutputJSONResponse, o.Out, o.Timeout, o.Verbose)

	if err != nil {
		return nil, nil, err
	}

	children, err := jsonParsed.S("data").Children()
	if err == gabs.ErrNotObjOrArray {
		return nil, api.GetPaging(jsonParsed), nil
	}
	return children, api.GetPaging(jsonParsed), err
}

func runListEdgeRouters(roleFilters []string, roleSemantic string, options *api.Options) error {
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
	children, paging, err := api.ListEntitiesOfType(util.EdgeAPI, "edge-routers", params, options.OutputJSONResponse, options.Out, options.Timeout, options.Verbose)
	if err != nil {
		return err
	}

	return outputEdgeRouters(options, children, paging)
}

func outputEdgeRouters(o *api.Options, children []*gabs.Container, pagingInfo *api.Paging) error {
	if o.OutputJSONResponse {
		return nil
	}

	t := table.NewWriter()
	t.SetStyle(table.StyleRounded)
	t.AppendHeader(table.Row{"ID", "Name", "Online", "Allow Transit", "Cost", "Attributes"})

	for _, entity := range children {
		wrapper := api.Wrap(entity)
		t.AppendRow(table.Row{
			wrapper.String("id"),
			wrapper.String("name"),
			wrapper.Bool("isOnline"),
			!wrapper.Bool("noTraversal"),
			wrapper.Float64("cost"),
			strings.Join(wrapper.StringSlice("roleAttributes"), "\n")})
	}
	api.RenderTable(o, t, pagingInfo)
	return nil
}

func runListEdgeRouterPolicies(o *api.Options) error {
	children, paging, err := listEntitiesWithOptions("edge-router-policies", o)
	if err != nil {
		return err
	}
	return outputEdgeRouterPolicies(o, children, paging)
}

func outputEdgeRouterPolicies(o *api.Options, children []*gabs.Container, pagingInfo *api.Paging) error {
	if o.OutputJSONResponse {
		return nil
	}

	t := table.NewWriter()
	t.SetStyle(table.StyleRounded)
	t.AppendHeader(table.Row{"ID", "Name", "Edge Router Roles", "Identity Roles"})

	for _, entity := range children {
		wrapper := api.Wrap(entity)
		identityRoles, err := mapRoleIdsToNames(entity, "identityRoles")
		if err != nil {
			return err
		}

		edgeRouterRoles, err := mapRoleIdsToNames(entity, "edgeRouterRoles")
		if err != nil {
			return err
		}

		t.AppendRow(table.Row{
			wrapper.String("id"),
			wrapper.String("name"),
			strings.Join(edgeRouterRoles, " "),
			strings.Join(identityRoles, " "),
		})
	}
	api.RenderTable(o, t, pagingInfo)
	return nil
}

func runListAuthenticators(o *api.Options) error {
	children, pagingInfo, err := listEntitiesWithOptions("authenticators", o)
	if err != nil {
		return err
	}
	return outputAuthenticators(o, children, pagingInfo)
}

func outputAuthenticators(o *api.Options, children []*gabs.Container, pagingInfo *api.Paging) error {
	if o.OutputJSONResponse {
		return nil
	}

	t := table.NewWriter()
	t.SetStyle(table.StyleRounded)
	t.AppendHeader(table.Row{"ID", "Method", "Identity Id", "Identity Name", "Username/Fingerprint", "Ca Id"})

	for _, entity := range children {
		id, _ := entity.Path("id").Data().(string)
		method := entity.Path("method").Data().(string)
		identityId := entity.Path("identityId").Data().(string)
		identityName := entity.Path("identity.name").Data().(string)

		caId := ""
		if entity.Exists("caId") {
			caId = entity.Path("caId").Data().(string)
		}

		printOrName := ""
		if entity.Exists("username") {
			printOrName = entity.Path("username").Data().(string)
		} else if entity.Exists("fingerprint") {
			printOrName = entity.Path("fingerprint").Data().(string)
		}

		t.AppendRow(table.Row{id, method, identityId, identityName, printOrName, caId})
	}
	api.RenderTable(o, t, pagingInfo)
	return nil
}

func runListEnrollments(o *api.Options) error {
	children, pagingInfo, err := listEntitiesWithOptions("enrollments", o)
	if err != nil {
		return err
	}
	return outputEnrollments(o, children, pagingInfo)
}

func outputEnrollments(o *api.Options, children []*gabs.Container, pagingInfo *api.Paging) error {
	if o.OutputJSONResponse {
		return nil
	}

	t := table.NewWriter()
	t.SetStyle(table.StyleRounded)
	t.AppendHeader(table.Row{"ID", "Method", "Identity Id", "Identity Name", "Expires At", "Token", "JWT"})

	for _, entity := range children {
		id, _ := entity.Path("id").Data().(string)
		method := entity.Path("method").Data().(string)
		identityId := entity.Path("identityId").Data().(string)
		identityName := entity.Path("identity.name").Data().(string)
		expiresAt := entity.Path("expiresAt").Data().(string)
		token := entity.Path("token").Data().(string)
		jwt := "See json"

		t.AppendRow(table.Row{id, method, identityId, identityName, expiresAt, token, jwt})
	}
	api.RenderTable(o, t, pagingInfo)
	return nil
}

func runListTerminators(o *api.Options) error {
	children, pagingInfo, err := listEntitiesWithOptions("terminators", o)
	if err != nil {
		return err
	}
	return outputTerminators(o, children, pagingInfo)
}

func outputTerminators(o *api.Options, children []*gabs.Container, pagingInfo *api.Paging) error {
	if o.OutputJSONResponse {
		return nil
	}

	t := table.NewWriter()
	t.SetStyle(table.StyleRounded)
	t.AppendHeader(table.Row{"ID", "Service", "Router", "Binding", "Address", "Identity", "Cost", "Precedence", "Dynamic Cost"})

	for _, entity := range children {
		id, _ := entity.Path("id").Data().(string)
		service := entity.Path("service.name").Data().(string)
		router := entity.Path("router.name").Data().(string)
		binding := entity.Path("binding").Data().(string)
		address := entity.Path("address").Data().(string)
		identity := entity.Path("identity").Data().(string)
		staticCost := entity.Path("cost").Data().(float64)
		precedence := entity.Path("precedence").Data().(string)
		dynamicCost := entity.Path("dynamicCost").Data().(float64)

		t.AppendRow(table.Row{id, service, router, binding, address, identity, staticCost, precedence, dynamicCost})
	}
	api.RenderTable(o, t, pagingInfo)
	return nil
}

func runListServices(asIdentity string, configTypes []string, roleFilters []string, roleSemantic string, options *api.Options) error {
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
	children, pagingInfo, err := ListEntitiesOfType("services", params, options.OutputJSONResponse, options.Out, options.Timeout, options.Verbose)
	if err != nil {
		return err
	}
	return outputServices(options, children, pagingInfo)
}

func outputServices(o *api.Options, children []*gabs.Container, pagingInfo *api.Paging) error {
	if o.OutputJSONResponse {
		return nil
	}

	t := table.NewWriter()
	t.SetStyle(table.StyleRounded)
	t.AppendHeader(table.Row{"ID", "Name", "Encryption Required", "Terminator Strategy", "Attributes"})
	t.SetColumnConfigs([]table.ColumnConfig{
		{Number: 3, WidthMax: 10},
	})

	for _, entity := range children {
		wrapper := api.Wrap(entity)
		t.AppendRow(table.Row{
			wrapper.String("id"),
			wrapper.String("name"),
			wrapper.Bool("encryptionRequired"),
			wrapper.String("terminatorStrategy"),
			strings.Join(wrapper.StringSlice("roleAttributes"), "\n")})
	}
	api.RenderTable(o, t, pagingInfo)

	return nil
}

func outputServiceConfigs(o *api.Options, children []*gabs.Container, pagingInfo *api.Paging) error {
	if o.OutputJSONResponse {
		return nil
	}

	t := table.NewWriter()
	t.SetStyle(table.StyleRounded)
	t.AppendHeader(table.Row{"Service Name", "Config Name"})

	for _, entity := range children {
		wrapper := api.Wrap(entity)
		t.AppendRow(table.Row{
			wrapper.String("service.name"),
			wrapper.String("config.name"),
		})
	}
	api.RenderTable(o, t, pagingInfo)
	return nil
}

func runListServiceEdgeRouterPolices(o *api.Options) error {
	children, pagingInfo, err := listEntitiesWithOptions("service-edge-router-policies", o)
	if err != nil {
		return err
	}
	return outputServiceEdgeRouterPolicies(o, children, pagingInfo)
}

func outputServiceEdgeRouterPolicies(o *api.Options, children []*gabs.Container, pagingInfo *api.Paging) error {
	if o.OutputJSONResponse {
		return nil
	}

	t := table.NewWriter()
	t.SetStyle(table.StyleRounded)
	t.AppendHeader(table.Row{"ID", "Name", "Service Roles", "Edge Router Roles"})

	for _, entity := range children {
		wrapper := api.Wrap(entity)
		serviceRoles, err := mapRoleIdsToNames(entity, "serviceRoles")
		if err != nil {
			return err
		}

		edgeRouterRoles, err := mapRoleIdsToNames(entity, "edgeRouterRoles")
		if err != nil {
			return err
		}

		t.AppendRow(table.Row{
			wrapper.String("id"),
			wrapper.String("name"),
			strings.Join(serviceRoles, " "),
			strings.Join(edgeRouterRoles, " "),
		})
	}
	api.RenderTable(o, t, pagingInfo)
	return nil
}

func runListServicePolices(o *api.Options) error {
	children, pagingInfo, err := listEntitiesWithOptions("service-policies", o)
	if err != nil {
		return err
	}
	return outputServicePolicies(o, children, pagingInfo)
}

func outputServicePolicies(o *api.Options, children []*gabs.Container, pagingInfo *api.Paging) error {
	if o.OutputJSONResponse {
		return nil
	}

	t := table.NewWriter()
	t.SetStyle(table.StyleRounded)
	t.AppendHeader(table.Row{"ID", "Name", "Semantic", "Service Roles", "Identity Roles", "Posture Check Roles"})

	for _, entity := range children {
		wrapper := api.Wrap(entity)

		serviceRoles, err := mapRoleIdsToNames(entity, "serviceRoles")
		if err != nil {
			return err
		}

		identityRoles, err := mapRoleIdsToNames(entity, "identityRoles")
		if err != nil {
			return err
		}

		postureCheckRoles, err := mapRoleIdsToNames(entity, "postureCheckRoles")
		if err != nil {
			return err
		}

		t.AppendRow(table.Row{
			wrapper.String("id"),
			wrapper.String("name"),
			wrapper.String("semantic"),
			strings.Join(serviceRoles, " "),
			strings.Join(identityRoles, " "),
			strings.Join(postureCheckRoles, " "),
		})
	}
	api.RenderTable(o, t, pagingInfo)
	return nil
}

func mapRoleIdsToNames(c *gabs.Container, path string) ([]string, error) {
	displayValues := map[string]string{}
	if displayValuesArr, err := c.Path(path + "Display").Children(); err == nil {
		for _, val := range displayValuesArr {
			role := val.S("role").Data().(string)
			name := val.S("name").Data().(string)
			displayValues[role] = name
		}
	} else {
		return nil, errors.Wrapf(err, "unable to get display values in %v", path+"Display")
	}

	jsonValues := c.Path(path).Data()
	if jsonValues == nil {
		return nil, nil
	}

	values := jsonValues.([]interface{})

	var result []string
	for _, val := range values {
		str := val.(string)
		if strings.HasPrefix(str, "@") {
			if name, found := displayValues[str]; found {
				result = append(result, name)
			} else {
				fmt.Printf("no display name provided for %v\n", str)
				result = append(result, str)
			}
		} else {
			result = append(result, str)
		}
	}
	return result, nil
}

// runListIdentities implements the command to list identities
func runListIdentities(roleFilters []string, roleSemantic string, options *api.Options) error {
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
	children, pagingInfo, err := ListEntitiesOfType("identities", params, options.OutputJSONResponse, options.Out, options.Timeout, options.Verbose)
	if err != nil {
		return err
	}
	return outputIdentities(options, children, pagingInfo)
}

// outputIdentities implements the command to list identities
func outputIdentities(o *api.Options, children []*gabs.Container, pagingInfo *api.Paging) error {
	if o.OutputJSONResponse {
		return nil
	}

	t := table.NewWriter()
	t.SetStyle(table.StyleRounded)
	t.AppendHeader(table.Row{"ID", "Name", "Type", "Attributes"})

	for _, entity := range children {
		wrapper := api.Wrap(entity)
		t.AppendRow(table.Row{
			wrapper.String("id"),
			wrapper.String("name"),
			wrapper.String("type.name"),
			strings.Join(wrapper.StringSlice("roleAttributes"), ",")})
	}
	api.RenderTable(o, t, pagingInfo)

	return nil
}

func outputPostureCheck(o *api.Options, entity *gabs.Container) error {
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

func outputPostureChecks(o *api.Options, children []*gabs.Container, pagingInfo *api.Paging) error {
	if o.OutputJSONResponse {
		return nil
	}

	for _, entity := range children {
		if err := outputPostureCheck(o, entity); err != nil {
			return err
		}
	}
	pagingInfo.Output(o)

	return nil
}

func runListCAs(o *api.Options) error {
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
	pagingInfo.Output(o)

	_, _ = fmt.Fprint(o.Out, "\nFlags: (V) Verified, (A) AutoCa Enrollment, (O) OttCA Enrollment, (E) Authentication Enabled\n\n")

	return nil
}

func runListConfigTypes(o *api.Options) error {
	children, pagingInfo, err := listEntitiesWithOptions("config-types", o)
	if err != nil {
		return err
	}

	if o.OutputJSONResponse {
		return nil
	}

	t := table.NewWriter()
	t.SetStyle(table.StyleRounded)
	t.AppendHeader(table.Row{"ID", "Name", "Schema"})

	for _, entity := range children {
		wrapper := api.Wrap(entity)
		t.AppendRow(table.Row{
			wrapper.String("id"),
			wrapper.String("name"),
		})
	}
	api.RenderTable(o, t, pagingInfo)

	return nil
}

func runListConfigs(o *api.Options) error {
	children, pagingInfo, err := listEntitiesWithOptions("configs", o)
	if err != nil {
		return err
	}
	return outputConfigs(o, children, pagingInfo)
}

func outputConfigs(o *api.Options, children []*gabs.Container, pagingInfo *api.Paging) error {
	if o.OutputJSONResponse {
		return nil
	}

	t := table.NewWriter()
	t.SetStyle(table.StyleRounded)
	t.AppendHeader(table.Row{"ID", "Name", "Config Type"})

	for _, entity := range children {
		wrapper := api.Wrap(entity)
		t.AppendRow(table.Row{
			wrapper.String("id"),
			wrapper.String("name"),
			wrapper.String("configType.name"),
		})
	}
	api.RenderTable(o, t, pagingInfo)

	return nil
}

func runListApiSessions(o *api.Options) error {
	children, pagingInfo, err := listEntitiesWithOptions("api-sessions", o)
	if err != nil {
		return err
	}

	if o.OutputJSONResponse {
		return nil
	}

	t := table.NewWriter()
	t.SetStyle(table.StyleRounded)
	t.AppendHeader(table.Row{"ID", "Token", "Identity Name"})

	for _, entity := range children {
		wrapper := api.Wrap(entity)
		t.AppendRow(table.Row{
			wrapper.String("id"),
			wrapper.String("token"),
			wrapper.String("identity.name"),
		})
	}
	api.RenderTable(o, t, pagingInfo)

	return nil
}

func runListSessions(o *api.Options) error {
	children, pagingInfo, err := listEntitiesWithOptions("sessions", o)

	if err != nil {
		return err
	}

	if o.OutputJSONResponse {
		return nil
	}

	t := table.NewWriter()
	t.SetStyle(table.StyleRounded)
	t.AppendHeader(table.Row{"ID", "API Session ID", "Service Name", "Type"})

	for _, entity := range children {
		wrapper := api.Wrap(entity)
		t.AppendRow(table.Row{
			wrapper.String("id"),
			wrapper.String("apiSession.id"),
			wrapper.String("service.name"),
			wrapper.String("type"),
		})
	}
	api.RenderTable(o, t, pagingInfo)

	return nil
}

func runListTransitRouters(o *api.Options) error {
	children, pagingInfo, err := listEntitiesWithOptions("transit-routers", o)

	if err != nil {
		return err
	}

	if o.OutputJSONResponse {
		return nil
	}

	t := table.NewWriter()
	t.SetStyle(table.StyleRounded)
	t.AppendHeader(table.Row{"ID", "Name"})

	for _, entity := range children {
		wrapper := api.Wrap(entity)
		t.AppendRow(table.Row{
			wrapper.String("id"),
			wrapper.String("name"),
		})
	}
	api.RenderTable(o, t, pagingInfo)

	return nil
}

func runListEdgeRouterRoleAttributes(o *api.Options) error {
	return runListRoleAttributes("edge-router", o)
}

func runListIdentityRoleAttributes(o *api.Options) error {
	return runListRoleAttributes("identity", o)
}

func runListServiceRoleAttributes(o *api.Options) error {
	return runListRoleAttributes("service", o)
}

func runListRoleAttributes(entityType string, o *api.Options) error {
	children, pagingInfo, err := listEntitiesWithOptions(entityType+"-role-attributes", o)

	if err != nil {
		return err
	}

	if o.OutputJSONResponse {
		return nil
	}

	t := table.NewWriter()
	t.SetStyle(table.StyleRounded)
	t.AppendHeader(table.Row{"Role Attribute"})

	for _, entity := range children {
		t.AppendRow(table.Row{
			fmt.Sprintf("%v", entity.Data()),
		})
	}
	api.RenderTable(o, t, pagingInfo)

	return nil
}

func runListChilden(parentType, childType string, o *api.Options, outputF outputFunction) error {
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

func runListPostureChecks(o *api.Options) error {
	children, pagingInfo, err := listEntitiesWithOptions("posture-checks", o)

	if err != nil {
		return err
	}

	if err := outputPostureChecks(o, children, pagingInfo); err != nil {
		return err
	}

	return err
}

func runListSummary(o *api.Options) error {
	jsonParsed, err := util.EdgeControllerList("summary", url.Values{}, o.OutputJSONResponse, o.Out, o.Timeout, o.Verbose)
	if err != nil {
		return err
	}
	if o.OutputJSONResponse {
		return nil
	}

	data := jsonParsed.S("data")
	children, err := data.ChildrenMap()
	if err != nil {
		return err
	}

	var keys []string
	for k := range children {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	t := table.NewWriter()
	t.SetStyle(table.StyleRounded)
	t.SetColumnConfigs([]table.ColumnConfig{{Number: 2, Align: text.AlignRight}})
	t.AppendHeader(table.Row{"Entity Type", "Count"})

	for _, k := range keys {
		v := children[k]
		t.AppendRow(table.Row{k, fmt.Sprintf("%v", v.Data())})
	}
	api.RenderTable(o, t, nil)

	return nil
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

func runListPostureCheckTypes(o *api.Options) error {
	children, pagingInfo, err := listEntitiesWithOptions("posture-check-types", o)

	if err != nil {
		return err
	}

	if o.OutputJSONResponse {
		return nil
	}

	t := table.NewWriter()
	t.SetStyle(table.StyleRounded)
	t.AppendHeader(table.Row{"ID", "Operating Systems"})

	for _, entity := range children {
		wrapper := api.Wrap(entity)
		operatingSystems, _ := entity.Path("operatingSystems").Children()
		osInfo := postureCheckOsToStrings(operatingSystems)

		t.AppendRow(table.Row{
			wrapper.String("id"),
			strings.Join(osInfo, ","),
		})
	}
	api.RenderTable(o, t, pagingInfo)

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
