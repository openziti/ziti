/*
	Copyright 2019 Netfoundry, Inc.

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
	"github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/common"
	cmdutil "github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/helpers"
	"github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/util"
	"fmt"
	"github.com/Jeffail/gabs"
	"github.com/spf13/cobra"
	"io"
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

	cmd.AddCommand(newListCmdForEntityType("app-wans", runListAppWans, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("cas", runListCAs, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("clusters", runListClusters, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("edge-routers", runListEdgeRouters, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("gateways", runListEdgeRouters, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("identities", runListIdentities, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("services", runListServices, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("sessions", runListSessions, newOptions()))
	cmd.AddCommand(newListCmdForEntityType("network-sessions", runListNetworkSessions, newOptions()))

	return cmd
}

type listCommandRunner func(*commonOptions) error

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

	jsonParsed, err := util.EdgeControllerListEntities(session.Host, session.Cert, session.Token, entityType, filter, outputJSON)

	if err != nil {
		return nil, err
	}

	return jsonParsed.S("data").Children()
}

func runListClusters(o *commonOptions) error {

	children, err := listEntitiesOfTypeWithOptionalFilter("clusters", o)

	if err != nil {
		return err
	}

	for _, cluster := range children {
		id, _ := cluster.Path("id").Data().(string)
		name, _ := cluster.Path("name").Data().(string)
		if _, err = fmt.Fprintf(o.Out, "id: %v    name: %v\n", id, name); err != nil {
			panic(err)
		}
	}

	return err
}

func runListEdgeRouters(o *commonOptions) error {

	children, err := listEntitiesOfTypeWithOptionalFilter("edge-routers", o)

	if err != nil {
		return err
	}

	for _, entity := range children {
		id, _ := entity.Path("id").Data().(string)
		name, _ := entity.Path("name").Data().(string)
		cluster, _ := entity.Path("cluster.id").Data().(string)
		fmt.Fprintf(o.Out, "id: %v    name: %v    cluster-id: %v\n", id, name, cluster)
	}

	return err
}

func runListServices(o *commonOptions) error {

	children, err := listEntitiesOfTypeWithOptionalFilter("services", o)

	if err != nil {
		return err
	}

	for _, entity := range children {
		id, _ := entity.Path("id").Data().(string)
		name, _ := entity.Path("name").Data().(string)
		cluster := entity.Path("clusters.name").String()
		fmt.Fprintf(o.Out, "id: %v    name: %v    clusters: %v\n", id, name, cluster)
	}

	return err
}

// runListIdentities implements the command to list identities
func runListIdentities(o *commonOptions) error {

	children, err := listEntitiesOfTypeWithOptionalFilter("identities", o)

	if err != nil {
		return err
	}

	for _, entity := range children {
		id, _ := entity.Path("id").Data().(string)
		name, _ := entity.Path("name").Data().(string)
		typeName, _ := entity.Path("type.name").Data().(string)
		fmt.Fprintf(o.Out, "id: %v    name: %v    type: %v\n", id, name, typeName)
	}

	return err
}

func runListAppWans(o *commonOptions) error {

	children, err := listEntitiesOfTypeWithOptionalFilter("app-wans", o)

	if err != nil {
		return err
	}

	for _, entity := range children {
		id, _ := entity.Path("id").Data().(string)
		name, _ := entity.Path("name").Data().(string)
		fmt.Fprintf(o.Out, "id: %v    name: %v\n", id, name)
	}

	return err
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
		fmt.Fprintf(o.Out, "id: %v    name: %v    cluster-id: %v\n", id, name, cluster)
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
		sessionToken, _ := entity.Path("token").Data().(string)
		identityName, _ := entity.Path("identity.name").Data().(string)
		fmt.Fprintf(o.Out, "id: %v    token: %v    identity: %v\n", id, sessionToken, identityName)
	}

	return err
}

func runListNetworkSessions(o *commonOptions) error {

	children, err := listEntitiesOfTypeWithOptionalFilter("network-sessions", o)

	if err != nil {
		return err
	}

	for _, entity := range children {
		id, _ := entity.Path("id").Data().(string)
		sessionId, _ := entity.Path("session.id").Data().(string)
		serviceName, _ := entity.Path("service.name").Data().(string)
		hosting, _ := entity.Path("hosting").Data().(bool)
		fmt.Fprintf(o.Out, "id: %v    sessionId: %v    serviceName: %v     hosting: %v\n", id, sessionId, serviceName, hosting)
	}

	return err
}
