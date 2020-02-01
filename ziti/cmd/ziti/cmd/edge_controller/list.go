/*
	Copyright 2019 NetFoundry, Inc.

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
	cmd.AddCommand(newListCmdForEntityType("edge-routers", runListEdgeRouters, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("edge-router-policies", runListEdgeRouterPolicies, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("gateways", runListEdgeRouters, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("identities", runListIdentities, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("services", runListServices, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("service-edge-router-policies", runListServiceEdgeRouterPolices, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("service-policies", runListServicePolices, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("sessions", runListSessions, newOptions()))

	configTypeListRootCmd := newEntityListRootCmd("config-type")
	configTypeListRootCmd.AddCommand(newSubListCmdForEntityType("config-type", "configs", outputConfigs, newOptions()))

	edgeRouterListRootCmd := newEntityListRootCmd("edge-router")
	edgeRouterListRootCmd.AddCommand(newSubListCmdForEntityType("edge-router", "edge-router-policies", outputEdgeRouterPolicies, newOptions()))
	edgeRouterListRootCmd.AddCommand(newSubListCmdForEntityType("edge-router", "service-edge-router-polices", outputServiceEdgeRouterPolicies, newOptions()))

	edgeRouterPolicyListRootCmd := newEntityListRootCmd("edge-router-policy")
	edgeRouterPolicyListRootCmd.AddCommand(newSubListCmdForEntityType("edge-router-policies", "edge-routers", outputEdgeRouters, newOptions()))
	edgeRouterPolicyListRootCmd.AddCommand(newSubListCmdForEntityType("edge-router-policies", "identities", outputIdentities, newOptions()))

	identityListRootCmd := newEntityListRootCmd("identity")
	identityListRootCmd.AddCommand(newSubListCmdForEntityType("identities", "edge-router-policies", outputEdgeRouterPolicies, newOptions()))
	identityListRootCmd.AddCommand(newSubListCmdForEntityType("identities", "service-policies", outputServicePolicies, newOptions()))

	serviceListRootCmd := newEntityListRootCmd("service")
	serviceListRootCmd.AddCommand(newSubListCmdForEntityType("services", "configs", outputConfigs, newOptions()))
	serviceListRootCmd.AddCommand(newSubListCmdForEntityType("services", "service-policies", outputServicePolicies, newOptions()))

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

// newSubListCmdForEntityType creates the list command for the given entity type
func newSubListCmdForEntityType(entityType string, subType string, outputF outputFunction, options *commonOptions) *cobra.Command {
	desc := fmt.Sprintf("lists %v related to a %v instanced managed by the Ziti Edge Controller", subType, entityType)
	cmd := &cobra.Command{
		Use:   fmt.Sprintf("%v <id or name>", subType),
		Short: desc,
		Long:  desc,
		Args:  cobra.ExactArgs(1),
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
func listEntitiesOfType(entityType string, options *commonOptions) ([]*gabs.Container, error) {
	return filterEntitiesOfType(entityType, "", options.OutputJSONResponse)
}

func listEntitiesOfTypeWithOptionalFilter(entityType string, options *commonOptions) ([]*gabs.Container, error) {
	filter := ""
	if len(options.Args) > 0 {
		filter = options.Args[0]
	}
	return filterEntitiesOfType(entityType, filter, options.OutputJSONResponse)
}

// listEntitiesOfType queries the Ziti Controller for entities of the given type
func filterEntitiesOfType(entityType string, filter string, outputJSON bool) ([]*gabs.Container, error) {
	session := &session{}
	err := session.Load()

	if err != nil {
		return nil, err
	}

	if session.Host == "" {
		return nil, fmt.Errorf("host not specififed in cli config file. Exiting")
	}

	jsonParsed, err := util.EdgeControllerListEntities(session, entityType, filter, outputJSON)

	if err != nil {
		return nil, err
	}

	return jsonParsed.S("data").Children()
}

// listEntitiesOfType queries the Ziti Controller for entities of the given type
func filterSubEntitiesOfType(entityType, subType, entityId, filter string, outputJSON bool) ([]*gabs.Container, error) {
	session := &session{}
	err := session.Load()

	if err != nil {
		return nil, err
	}

	if session.Host == "" {
		return nil, fmt.Errorf("host not specififed in cli config file. Exiting")
	}

	jsonParsed, err := util.EdgeControllerListSubEntities(session, entityType, subType, entityId, filter, outputJSON)

	if err != nil {
		return nil, err
	}

	return jsonParsed.S("data").Children()
}

func runListEdgeRouters(o *commonOptions) error {
	children, err := listEntitiesOfTypeWithOptionalFilter("edge-routers", o)
	if err != nil {
		return err
	}
	return outputEdgeRouters(o, children)
}

func outputEdgeRouters(o *commonOptions, children []*gabs.Container) error {
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
	children, err := listEntitiesOfTypeWithOptionalFilter("edge-router-policies", o)
	if err != nil {
		return err
	}
	return outputEdgeRouterPolicies(o, children)
}

func outputEdgeRouterPolicies(o *commonOptions, children []*gabs.Container) error {
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

func runListServices(o *commonOptions) error {
	children, err := listEntitiesOfTypeWithOptionalFilter("services", o)
	if err != nil {
		return err
	}
	return outputServices(o, children)
}

func outputServices(o *commonOptions, children []*gabs.Container) error {
	for _, entity := range children {
		id, _ := entity.Path("id").Data().(string)
		name, _ := entity.Path("name").Data().(string)
		edgeRouterRoles := entity.Path("edgeRouterRoles").String()
		_, err := fmt.Fprintf(o.Out, "id: %v    name: %v    edge router roles: %v\n", id, name, edgeRouterRoles)
		if err != nil {
			return err
		}
	}

	return nil
}

func runListServiceEdgeRouterPolices(o *commonOptions) error {
	children, err := listEntitiesOfTypeWithOptionalFilter("service-edge-router-policies", o)
	if err != nil {
		return err
	}
	return outputServiceEdgeRouterPolicies(o, children)
}

func outputServiceEdgeRouterPolicies(o *commonOptions, children []*gabs.Container) error {
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
	children, err := listEntitiesOfTypeWithOptionalFilter("service-policies", o)
	if err != nil {
		return err
	}
	return outputServicePolicies(o, children)
}

func outputServicePolicies(o *commonOptions, children []*gabs.Container) error {
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
func runListIdentities(o *commonOptions) error {
	children, err := listEntitiesOfTypeWithOptionalFilter("identities", o)
	if err != nil {
		return err
	}
	return outputIdentities(o, children)
}

// outputIdentities implements the command to list identities
func outputIdentities(o *commonOptions, children []*gabs.Container) error {
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
	children, err := listEntitiesOfTypeWithOptionalFilter("cas", o)
	if err != nil {
		return err
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
	children, err := listEntitiesOfTypeWithOptionalFilter("config-types", o)
	if err != nil {
		return err
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
	children, err := listEntitiesOfTypeWithOptionalFilter("configs", o)
	if err != nil {
		return err
	}
	return outputConfigs(o, children)
}

func outputConfigs(o *commonOptions, children []*gabs.Container) error {
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
	children, err := listEntitiesOfTypeWithOptionalFilter("api-sessions", o)
	if err != nil {
		return err
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
	children, err := listEntitiesOfTypeWithOptionalFilter("sessions", o)

	if err != nil {
		return err
	}

	for _, entity := range children {
		id, _ := entity.Path("id").Data().(string)
		sessionId, _ := entity.Path("apiSession.id").Data().(string)
		serviceName, _ := entity.Path("service.name").Data().(string)
		hosting, _ := entity.Path("hosting").Data().(bool)
		if _, err := fmt.Fprintf(o.Out, "id: %v    sessionId: %v    serviceName: %v     hosting: %v\n", id, sessionId, serviceName, hosting); err != nil {
			return err
		}
	}

	return err
}

func runListChilden(parentType, childType string, o *commonOptions, outputF outputFunction) error {
	idOrName := o.Args[0]
	serviceId, err := mapNameToID(parentType, idOrName)
	if err != nil {
		return err
	}

	children, err := filterSubEntitiesOfType(parentType, childType, serviceId, "", o.OutputJSONResponse)
	if err != nil {
		return err
	}
	return outputF(o, children)
}
