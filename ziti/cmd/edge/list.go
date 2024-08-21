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
	"bytes"
	"fmt"
	"github.com/go-openapi/runtime"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/openziti/edge-api/rest_management_api_client/auth_policy"
	"github.com/openziti/edge-api/rest_management_api_client/external_jwt_signer"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/util"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/Jeffail/gabs"
	"github.com/openziti/edge-api/rest_management_api_client/certificate_authority"
	"github.com/openziti/edge-api/rest_model"
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
	cmd.AddCommand(newListCmdForEntityType("auth-policies", runListAuthPolicies, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("cas", runListCAs, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("config-types", runListConfigTypes, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("configs", runListConfigs, newOptions()))
	cmd.AddCommand(newListEdgeRoutersCmd(newOptions()))
	cmd.AddCommand(newListCmdForEntityType("edge-router-policies", runListEdgeRouterPolicies, newOptions(), "erps"))
	cmd.AddCommand(newListCmdForEntityType("enrollments", runListEnrollments, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("ext-jwt-signers", runListExtJwtSigners, newOptions(), "external-jwt-signers"))
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
	cmd.AddCommand(newListCmdForEntityType("posture-check-role-attributes", runListPostureCheckRoleAttributes, newOptions()))

	cmd.AddCommand(newListCmdForEntityType("posture-checks", runListPostureChecks, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("posture-check-types", runListPostureCheckTypes, newOptions()))

	configTypeListRootCmd := newEntityListRootCmd("config-type")
	configTypeListRootCmd.AddCommand(newSubListCmdForEntityType("config-type", "configs", outputConfigs, newOptions()))

	configListRootCmd := newEntityListRootCmd("config")
	configListRootCmd.AddCommand(newSubListCmdForEntityType("configs", "services", outputServices, newOptions()))

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
		configListRootCmd,
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
			err := runListChildren(entityType, subType, options, outputF)
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

func runListAuthPolicies(options *api.Options) error {

	client, err := util.NewEdgeManagementClient(options)

	if err != nil {
		return err
	}

	var filter *string = nil

	if len(options.Args) > 0 {
		filter = &options.Args[0]
	}

	params := auth_policy.NewListAuthPoliciesParams()
	params.Filter = filter

	result, err := client.AuthPolicy.ListAuthPolicies(params, nil)

	if err != nil {
		return util.WrapIfApiError(err)
	}

	if options.OutputJSONResponse {
		return nil
	}

	payload := result.GetPayload()

	if payload == nil {
		return errors.New("unexpected empty response payload")
	}

	outTable := table.NewWriter()
	outTable.SetStyle(table.StyleRounded)
	outTable.Style().Options.SeparateRows = true

	rowConfigAutoMerge := table.RowConfig{AutoMerge: true, AutoMergeAlign: text.AlignLeft}

	outTable.SetColumnConfigs([]table.ColumnConfig{
		{Number: 1, AutoMerge: true, Align: text.AlignLeft},
		{Number: 2, AutoMerge: true, Align: text.AlignLeft},
		{Number: 3, AutoMerge: true, Align: text.AlignLeft},
		{Number: 4, AutoMerge: true, Align: text.AlignLeft},
		{Number: 5, AutoMerge: true, Align: text.AlignLeft},
	})

	outTable.AppendHeader(table.Row{"ID", "Name", "Section", "Type", "Config", "Config"}, rowConfigAutoMerge)

	for _, entity := range result.GetPayload().Data {
		id := *entity.ID
		name := *entity.Name

		maxAttempts := "0 (unlim)"

		if *entity.Primary.Updb.MaxAttempts != 0 {
			maxAttempts = fmt.Sprintf("%d", *entity.Primary.Updb.MaxAttempts)
		}

		lockout := "0 (forever)"

		if *entity.Primary.Updb.MaxAttempts != 0 {
			lockout = fmt.Sprintf("%d", *entity.Primary.Updb.LockoutDurationMinutes)
		}

		outTable.AppendRow(table.Row{id, name, "Primary", "CERT", "Allowed", *entity.Primary.Cert.Allowed}, rowConfigAutoMerge)
		outTable.AppendRow(table.Row{id, name, "Primary", "CERT", "Allowed Expired", *entity.Primary.Cert.AllowExpiredCerts}, rowConfigAutoMerge)
		outTable.AppendRow(table.Row{id, name, "Primary", "UPDB", "Allowed", *entity.Primary.Updb.Allowed}, rowConfigAutoMerge)
		outTable.AppendRow(table.Row{id, name, "Primary", "UPDB", "Max Attempts", maxAttempts}, rowConfigAutoMerge)
		outTable.AppendRow(table.Row{id, name, "Primary", "UPDB", "Lockout (M)", lockout}, rowConfigAutoMerge)
		outTable.AppendRow(table.Row{id, name, "Primary", "UPDB", "Min Password Len", *entity.Primary.Updb.MinPasswordLength}, rowConfigAutoMerge)
		outTable.AppendRow(table.Row{id, name, "Primary", "UPDB", "Require Mix Case", *entity.Primary.Updb.RequireMixedCase}, rowConfigAutoMerge)
		outTable.AppendRow(table.Row{id, name, "Primary", "UPDB", "Require Specials", *entity.Primary.Updb.RequireSpecialChar}, rowConfigAutoMerge)
		outTable.AppendRow(table.Row{id, name, "Primary", "UPDB", "Require Numbers", *entity.Primary.Updb.RequireNumberChar}, rowConfigAutoMerge)
		outTable.AppendRow(table.Row{id, name, "Primary", "EXT-JWT", "Allowed", *entity.Primary.ExtJWT.Allowed}, rowConfigAutoMerge)

		if len(entity.Primary.ExtJWT.AllowedSigners) > 0 {
			for _, signerId := range entity.Primary.ExtJWT.AllowedSigners {
				outTable.AppendRow(table.Row{id, name, "Primary", "EXT-JWT", "Allowed Signers", signerId}, rowConfigAutoMerge)
			}
		} else {
			outTable.AppendRow(table.Row{id, name, "Primary", "EXT-JWT", "Allowed Signers", "none"}, rowConfigAutoMerge)
		}

		outTable.AppendRow(table.Row{id, name, "Secondary", "TOTP MFA", "Required", *entity.Secondary.RequireTotp}, rowConfigAutoMerge)
		outTable.AppendRow(table.Row{id, name, "Secondary", "EXT-JWT", "Required Signer", stringz.OrEmpty(entity.Secondary.RequireExtJWTSigner)}, rowConfigAutoMerge)
	}

	pagingInfo := newPagingInfo(payload.Meta)
	api.RenderTable(options, outTable, pagingInfo)

	return nil
}

func runListExtJwtSigners(options *api.Options) error {
	client, err := util.NewEdgeManagementClient(options)

	if err != nil {
		return err
	}

	var filter *string = nil

	if len(options.Args) > 0 {
		filter = &options.Args[0]
	}

	params := external_jwt_signer.NewListExternalJWTSignersParams()
	params.Filter = filter

	result, err := client.ExternalJWTSigner.ListExternalJWTSigners(params, nil)

	if err != nil {
		return util.WrapIfApiError(err)
	}

	if options.OutputJSONResponse {
		return nil
	}

	payload := result.GetPayload()

	if payload == nil {
		return errors.New("unexpected empty response payload")
	}

	outTable := table.NewWriter()
	outTable.SetStyle(table.StyleRounded)
	outTable.Style().Options.SeparateRows = true

	rowConfigAutoMerge := table.RowConfig{AutoMerge: true, AutoMergeAlign: text.AlignLeft}

	outTable.SetColumnConfigs([]table.ColumnConfig{
		{Number: 1, AutoMerge: true, Align: text.AlignLeft},
		{Number: 2, AutoMerge: true, Align: text.AlignLeft},
	})

	outTable.AppendHeader(table.Row{"ID", "Name", "Config", "Config"}, rowConfigAutoMerge)

	for _, entity := range result.GetPayload().Data {
		id := *entity.ID
		name := *entity.Name
		audience := *entity.Audience
		isEnabled := *entity.Enabled
		claimsProperty := *entity.ClaimsProperty
		useExternalId := *entity.UseExternalID
		issuer := *entity.Issuer
		clientId := stringz.OrEmpty(entity.ClientID)
		scopes := strings.Join(entity.Scopes, ",")

		if entity.JwksEndpoint != nil {
			confType := "JWKS"
			urlStr := entity.JwksEndpoint.String()
			outTable.AppendRow(table.Row{id, name, "Audience", audience}, rowConfigAutoMerge)
			outTable.AppendRow(table.Row{id, name, "Claim Property", claimsProperty}, rowConfigAutoMerge)
			outTable.AppendRow(table.Row{id, name, "Enabled", isEnabled}, rowConfigAutoMerge)
			outTable.AppendRow(table.Row{id, name, "Issuer", issuer}, rowConfigAutoMerge)
			outTable.AppendRow(table.Row{id, name, "JWKS URL", urlStr}, rowConfigAutoMerge)
			outTable.AppendRow(table.Row{id, name, "Type", confType}, rowConfigAutoMerge)
			outTable.AppendRow(table.Row{id, name, "Use External Id", useExternalId}, rowConfigAutoMerge)
			outTable.AppendRow(table.Row{id, name, "ClientId", clientId}, rowConfigAutoMerge)
			outTable.AppendRow(table.Row{id, name, "Scopes", scopes}, rowConfigAutoMerge)
		} else {
			confType := "CERT"
			fingerprint := *entity.Fingerprint
			outTable.AppendRow(table.Row{id, name, "Audience", audience}, rowConfigAutoMerge)
			outTable.AppendRow(table.Row{id, name, "Claim Property", claimsProperty}, rowConfigAutoMerge)
			outTable.AppendRow(table.Row{id, name, "Enabled", isEnabled}, rowConfigAutoMerge)
			outTable.AppendRow(table.Row{id, name, "Issuer", issuer}, rowConfigAutoMerge)
			outTable.AppendRow(table.Row{id, name, "Fingerprint", fingerprint}, rowConfigAutoMerge)
			outTable.AppendRow(table.Row{id, name, "Type", confType}, rowConfigAutoMerge)
			outTable.AppendRow(table.Row{id, name, "Use External Id", useExternalId}, rowConfigAutoMerge)
			outTable.AppendRow(table.Row{id, name, "ClientId", clientId}, rowConfigAutoMerge)
			outTable.AppendRow(table.Row{id, name, "Scopes", scopes}, rowConfigAutoMerge)
		}

	}

	pagingInfo := newPagingInfo(payload.Meta)
	api.RenderTable(options, outTable, pagingInfo)

	return nil
}

func outputEnrollments(o *api.Options, children []*gabs.Container, pagingInfo *api.Paging) error {
	if o.OutputJSONResponse {
		return nil
	}

	t := table.NewWriter()
	t.SetStyle(table.StyleRounded)
	t.AppendHeader(table.Row{"ID", "Method", "Identity Id", "Identity Name", "Expires At", "Token", "JWT"})

	for _, entity := range children {
		id := api.GetJsonString(entity, "id")
		method := api.GetJsonString(entity, "method")
		identityId := api.GetJsonString(entity, "identityId")
		identityName := api.GetJsonString(entity, "identity.name")
		expiresAt := api.GetJsonString(entity, "expiresAt")
		token := api.GetJsonString(entity, "token")
		jwt := "See json"

		t.AppendRow(table.Row{id, method, identityId, identityName, expiresAt, token, jwt})
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
		if configTypes, err := mapNamesToIDs("config-types", *options, false, configTypes...); err != nil {
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
	t.AppendHeader(table.Row{"ID", "Name", "Type", "Attributes", "Auth-Policy"})

	for _, entity := range children {
		wrapper := api.Wrap(entity)
		authPolicy := wrapper.String("authPolicy.name")

		if authPolicy == "" {
			authPolicy = wrapper.String("authPolicyId")
		}

		t.AppendRow(table.Row{
			wrapper.String("id"),
			wrapper.String("name"),
			wrapper.String("type.name"),
			strings.Join(wrapper.StringSlice("roleAttributes"), ","),
			authPolicy})
	}
	api.RenderTable(o, t, pagingInfo)

	return nil
}

func getEllipsesString(val string, lead, lag int) string {
	total := lead + lag + 3

	if len(val) <= total {
		return val
	}

	return val[0:lead] + "..." + val[len(val)-lag:]
}

func strSliceToStr(strs []string, width int) string {
	builder := strings.Builder{}
	for i, str := range strs {
		if i != 0 {
			if i%width == 0 {
				builder.WriteRune('\n')
			} else {
				builder.WriteString(", ")
			}
		}

		builder.WriteString(str)
	}

	return builder.String()
}

func strSliceToStrEllipses(strs []string, width, lead, lag int) string {
	var ret []string
	for _, str := range strs {
		ret = append(ret, getEllipsesString(str, lead, lag))
	}
	return strSliceToStr(ret, width)
}

func WrapHardEllipses(str string, wrapLen int) string {
	newStr := text.WrapHard(str, wrapLen)

	if newStr != str {
		newStr = newStr[:len(newStr)-3] + "..."
	}

	return newStr
}

func outputPostureChecks(options *api.Options, children []*gabs.Container, pagingInfo *api.Paging) error {
	if options.OutputJSONResponse {
		return nil
	}

	outTable := table.NewWriter()
	outTable.SetStyle(table.StyleRounded)
	outTable.Style().Options.SeparateRows = true

	rowConfigAutoMerge := table.RowConfig{AutoMerge: true, AutoMergeAlign: text.AlignLeft}

	outTable.AppendHeader(table.Row{"ID", "Name", "Type", "Attributes", "Configuration", "Configuration", "Configuration"}, rowConfigAutoMerge)

	outTable.SetColumnConfigs([]table.ColumnConfig{
		{Number: 1, AutoMerge: true, Align: text.AlignLeft},
		{Number: 2, AutoMerge: true, Align: text.AlignLeft},
		{Number: 3, AutoMerge: true, WidthMax: 20, WidthMaxEnforcer: WrapHardEllipses, Align: text.AlignLeft},
		{Number: 4, AutoMerge: true, WidthMax: 20, WidthMaxEnforcer: WrapHardEllipses, Align: text.AlignLeft},
		{Number: 5, WidthMax: 20, WidthMaxEnforcer: WrapHardEllipses, Align: text.AlignLeft},
		{Number: 6, WidthMax: 50, WidthMaxEnforcer: WrapHardEllipses, Align: text.AlignLeft},
		{Number: 7, WidthMax: 50, WidthMaxEnforcer: WrapHardEllipses, Align: text.AlignLeft},
	})

	for i, entity := range children {
		json := entity.EncodeJSON()
		detail, err := rest_model.UnmarshalPostureCheckDetail(bytes.NewBuffer(json), runtime.JSONConsumer())

		id := stringz.OrEmpty(detail.ID())
		name := stringz.OrEmpty(detail.Name())
		timeout := "never"

		roleAttributes := strSliceToStr(*detail.RoleAttributes(), 1)

		if roleAttributes == "" {
			roleAttributes = "<none>"
		}

		typeStr := detail.TypeID()

		//defeat cell merging by adding a space every other row
		if i%2 == 0 {
			roleAttributes = roleAttributes + " "
			typeStr = typeStr + " "
		}

		if err != nil {
			msg := "Error unmarshalling index " + strconv.Itoa(i) + ": " + err.Error()
			_, _ = options.ErrOutputWriter().Write([]byte(msg))
		} else {
			switch detail.TypeID() {
			case "MFA":
				mfaDetail := detail.(*rest_model.PostureCheckMfaDetail)

				if mfaDetail.TimeoutSeconds > 0 {
					timeout = fmt.Sprintf("%ds", mfaDetail.TimeoutSeconds)
				}

				outTable.AppendRow(table.Row{
					id,
					name,
					typeStr,
					roleAttributes,
					"Timeout",
					timeout,
					timeout,
				}, rowConfigAutoMerge)

				outTable.AppendRow(table.Row{
					id,
					name,
					typeStr,
					roleAttributes,
					"Prompt On Wake",
					mfaDetail.PromptOnWake,
					mfaDetail.PromptOnWake,
				}, rowConfigAutoMerge)

				outTable.AppendRow(table.Row{
					id,
					name,
					typeStr,
					roleAttributes,
					"Prompt On Unlock",
					mfaDetail.PromptOnUnlock,
					mfaDetail.PromptOnUnlock,
				}, rowConfigAutoMerge)

			case "MAC":
				macDetail := detail.(*rest_model.PostureCheckMacAddressDetail)

				outTable.AppendRow(table.Row{
					id,
					name,
					typeStr,
					roleAttributes,
					"MAC Address",
					strSliceToStrEllipses(macDetail.MacAddresses, 3, 3, 3),
					strSliceToStrEllipses(macDetail.MacAddresses, 3, 3, 3),
				}, rowConfigAutoMerge)
			case "DOMAIN":
				domainDetails := detail.(*rest_model.PostureCheckDomainDetail)

				outTable.AppendRow(table.Row{
					id,
					name,
					typeStr,
					roleAttributes,
					"Windows Domain",
					strSliceToStr(domainDetails.Domains, 3),
					strSliceToStr(domainDetails.Domains, 3),
				}, rowConfigAutoMerge)
			case "OS":
				osDetails := detail.(*rest_model.PostureCheckOperatingSystemDetail)

				for _, os := range osDetails.OperatingSystems {
					osType := string(*os.Type)

					for _, version := range os.Versions {
						outTable.AppendRow(table.Row{
							id,
							name,
							typeStr,
							roleAttributes,
							osType,
							version,
							version,
						}, rowConfigAutoMerge)
					}
				}

			case "PROCESS_MULTI":
				procMultiDetail := detail.(*rest_model.PostureCheckProcessMultiDetail)

				outTable.AppendRow(table.Row{
					id,
					name,
					typeStr,
					roleAttributes,
					"Semantic",
					string(*procMultiDetail.Semantic),
					string(*procMultiDetail.Semantic),
				}, rowConfigAutoMerge)

				for _, process := range procMultiDetail.Processes {
					outTable.AppendRow(table.Row{
						id,
						name,
						typeStr,
						roleAttributes,
						"Path",
						stringz.OrEmpty(process.Path),
						stringz.OrEmpty(process.Path),
					}, rowConfigAutoMerge)

					outTable.AppendRow(table.Row{
						id,
						name,
						typeStr,
						roleAttributes,
						" ",
						"Hashes",
						strSliceToStrEllipses(process.Hashes, 3, 4, 4),
					})

					outTable.AppendRow(table.Row{
						id,
						name,
						typeStr,
						roleAttributes,
						" ",
						"Signers",
						strSliceToStrEllipses(process.SignerFingerprints, 1, 4, 4),
					})
				}

			case "PROCESS":
				procDetail := detail.(*rest_model.PostureCheckProcessDetail)

				outTable.AppendRow(table.Row{
					id,
					name,
					typeStr,
					roleAttributes,
					"Path",
					stringz.OrEmpty(procDetail.Process.Path),
					stringz.OrEmpty(procDetail.Process.Path),
				}, rowConfigAutoMerge)

				for _, hash := range procDetail.Process.Hashes {
					outTable.AppendRow(table.Row{
						id,
						name,
						typeStr,
						roleAttributes,
						"Hash",
						getEllipsesString(hash, 4, 4),
						getEllipsesString(hash, 4, 4),
					}, rowConfigAutoMerge)
				}

				outTable.AppendRow(table.Row{
					id,
					name,
					typeStr,
					roleAttributes,
					"Signer",
					getEllipsesString(procDetail.Process.SignerFingerprint, 4, 4),
					getEllipsesString(procDetail.Process.SignerFingerprint, 4, 4),
				}, rowConfigAutoMerge)
			}
		}
	}
	api.RenderTable(options, outTable, pagingInfo)
	return nil
}

func runListCAs(options *api.Options) error {
	client, err := util.NewEdgeManagementClient(options)

	if err != nil {
		return err
	}

	var filter *string = nil

	if len(options.Args) > 0 {
		filter = &options.Args[0]
	}

	context, cancelContext := options.TimeoutContext()
	defer cancelContext()

	result, err := client.CertificateAuthority.ListCas(&certificate_authority.ListCasParams{
		Filter:  filter,
		Context: context,
	}, nil)

	if err != nil {
		return util.WrapIfApiError(err)
	}

	if options.OutputJSONResponse {
		return nil
	}

	payload := result.GetPayload()

	if payload == nil {
		return errors.New("unexpected empty response payload")
	}

	outTable := table.NewWriter()
	outTable.SetStyle(table.StyleRounded)
	outTable.Style().Options.SeparateRows = true

	rowConfigAutoMerge := table.RowConfig{AutoMerge: true, AutoMergeAlign: text.AlignLeft}

	outTable.AppendHeader(table.Row{"ID", "Name", "Flags", "Token", "Fingerprint", "Configuration", "Configuration", "Configuration"}, rowConfigAutoMerge)

	outTable.SetColumnConfigs([]table.ColumnConfig{
		{Number: 1, AutoMerge: true, Align: text.AlignLeft},
		{Number: 2, AutoMerge: true, Align: text.AlignLeft},
		{Number: 3, AutoMerge: true, WidthMax: 20, WidthMaxEnforcer: WrapHardEllipses, Align: text.AlignLeft},
		{Number: 4, AutoMerge: true, WidthMax: 20, WidthMaxEnforcer: WrapHardEllipses, Align: text.AlignLeft},
		{Number: 5, AutoMerge: true, WidthMax: 20, WidthMaxEnforcer: WrapHardEllipses, Align: text.AlignLeft},
		{Number: 6, AutoMerge: true, WidthMax: 20, WidthMaxEnforcer: WrapHardEllipses, Align: text.AlignLeft},
		{Number: 7, WidthMax: 50, WidthMaxEnforcer: WrapHardEllipses, Align: text.AlignLeft},
		{Number: 8, WidthMax: 50, WidthMaxEnforcer: WrapHardEllipses, Align: text.AlignLeft},
	})

	for i, entity := range result.GetPayload().Data {
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

		identityRolesStr := strings.Join(identityRoles, ",")
		shortFingerprint := getEllipsesString(fingerprint, 4, 2)

		if token == "" {
			token = "-"
		}

		if shortFingerprint == "" {
			shortFingerprint = "-"
		}

		if i%2 == 0 {
			flags = flags + " "
			token = token + " "
			identityNameFormat = identityNameFormat + " "
			identityRolesStr = identityRolesStr + " "
			shortFingerprint = shortFingerprint + " "
		}

		outTable.AppendRow(table.Row{id, name, flags, token, shortFingerprint, "AutoCA", "Identity Name Format", identityNameFormat})
		outTable.AppendRow(table.Row{id, name, flags, token, shortFingerprint, "AutoCA", "Identity Roles", identityRolesStr})

		if entity.ExternalIDClaim != nil {
			outTable.AppendRow(table.Row{id, name, flags, token, shortFingerprint, "ExternalIdClaim", "Index", int64OrDefault(entity.ExternalIDClaim.Index)})
			outTable.AppendRow(table.Row{id, name, flags, token, shortFingerprint, "ExternalIdClaim", "Location", stringz.OrEmpty(entity.ExternalIDClaim.Location)})
			outTable.AppendRow(table.Row{id, name, flags, token, shortFingerprint, "ExternalIdClaim", "Matcher", stringz.OrEmpty(entity.ExternalIDClaim.Matcher)})
			outTable.AppendRow(table.Row{id, name, flags, token, shortFingerprint, "ExternalIdClaim", "Matcher Criteria", stringz.OrEmpty(entity.ExternalIDClaim.MatcherCriteria)})
			outTable.AppendRow(table.Row{id, name, flags, token, shortFingerprint, "ExternalIdClaim", "Parser", stringz.OrEmpty(entity.ExternalIDClaim.Parser)})
			outTable.AppendRow(table.Row{id, name, flags, token, shortFingerprint, "ExternalIdClaim", "Parser Criteria", stringz.OrEmpty(entity.ExternalIDClaim.ParserCriteria)})
		}
	}

	pagingInfo := newPagingInfo(payload.Meta)
	api.RenderTable(options, outTable, pagingInfo)

	_, _ = fmt.Fprint(options.Out, "\nFlags: (V) Verified, (A) AutoCa Enrollment, (O) OttCA Enrollment, (E) Authentication Enabled\n\n")

	return nil
}

func int64OrDefault(i *int64) int64 {
	if i == nil {
		return 0
	}
	return *i
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

func runListPostureCheckRoleAttributes(o *api.Options) error {
	return runListRoleAttributes("posture-check", o)
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

func runListChildren(parentType, childType string, o *api.Options, outputF outputFunction) error {
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
