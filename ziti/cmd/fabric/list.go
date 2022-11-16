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
	"fmt"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/util"
	"strings"

	"github.com/Jeffail/gabs"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/spf13/cobra"
)

// newListCmd creates a command object for the "controller list" command
func newListCmd(p common.OptionsProvider) *cobra.Command {
	listCmd := &cobra.Command{
		Use:     "list",
		Short:   "Lists various entities managed by the Ziti Controller",
		Aliases: []string{"ls"},
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			cmdhelper.CheckErr(err)
		},
	}

	newOptions := func() *api.Options {
		return &api.Options{CommonOptions: p()}
	}

	listCmd.AddCommand(newListCmdForEntityType("circuits", runListCircuits, newOptions()))
	listCmd.AddCommand(newListCmdForEntityType("links", runListLinks, newOptions()))
	listCmd.AddCommand(newListCmdForEntityType("routers", runListRouters, newOptions()))
	listCmd.AddCommand(newListCmdForEntityType("services", runListServices, newOptions()))
	listCmd.AddCommand(newListCmdForEntityType("terminators", runListTerminators, newOptions()))

	return listCmd
}

func listEntitiesWithOptions(entityType string, options *api.Options) ([]*gabs.Container, *api.Paging, error) {
	return api.ListEntitiesWithOptions(util.FabricAPI, entityType, options)
}

type listCommandRunner func(*api.Options) error

// newListCmdForEntityType creates the list command for the given entity type
func newListCmdForEntityType(entityType string, command listCommandRunner, options *api.Options, aliases ...string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     entityType + " <filter>?",
		Short:   "lists " + entityType + " managed by the Ziti Controller",
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

func runListCircuits(o *api.Options) error {
	children, pagingInfo, err := listEntitiesWithOptions("circuits", o)
	if err != nil {
		return err
	}
	return outputCircuits(o, children, pagingInfo)
}

func outputCircuits(o *api.Options, children []*gabs.Container, pagingInfo *api.Paging) error {
	if o.OutputJSONResponse {
		return nil
	}

	t := table.NewWriter()
	t.SetStyle(table.StyleRounded)
	t.AppendHeader(table.Row{"ID", "Client", "Service", "Terminator", "Path"})

	for _, entity := range children {
		id := api.GetJsonString(entity, "id")
		client := api.GetJsonString(entity, "clientId")
		service := api.GetJsonString(entity, "service.name")
		terminatorId := api.GetJsonString(entity, "terminator.id")

		path := strings.Builder{}

		nodes, err := getEntityRef(entity.Path("path.nodes"))
		if err != nil {
			return err
		}

		links, err := getEntityRef(entity.Path("path.links"))
		if err != nil {
			return err
		}

		if len(nodes) > 0 {
			path.WriteString("r/")
			path.WriteString(nodes[0].name)
			for idx, node := range nodes[1:] {
				link := links[idx]
				path.WriteString(" -> l/")
				path.WriteString(link.id)
				path.WriteString(" -> r/")
				path.WriteString(node.name)
			}
		}

		t.AppendRow(table.Row{id, client, service, terminatorId, path.String()})
	}

	api.RenderTable(o, t, pagingInfo)

	return nil
}

type entityRef struct {
	id   string
	name string
}

func getEntityRef(c *gabs.Container) ([]*entityRef, error) {
	if c == nil || c.Data() == nil {
		return nil, nil
	}
	children, err := c.Children()
	if err != nil {
		return nil, err
	}

	var result []*entityRef

	for _, child := range children {
		id := api.GetJsonString(child, "id")
		name := api.GetJsonString(child, "name")
		result = append(result, &entityRef{
			id:   id,
			name: name,
		})
	}
	return result, nil
}

func runListLinks(o *api.Options) error {
	children, pagingInfo, err := listEntitiesWithOptions("links", o)
	if err != nil {
		return err
	}
	return outputLinks(o, children, pagingInfo)
}

func outputLinks(o *api.Options, children []*gabs.Container, pagingInfo *api.Paging) error {
	if o.OutputJSONResponse {
		return nil
	}

	t := table.NewWriter()
	t.SetStyle(table.StyleRounded)
	columnConfigs := []table.ColumnConfig{
		{Number: 5, Align: text.AlignRight},
		{Number: 6, Align: text.AlignRight},
		{Number: 8, Align: text.AlignRight},
	}
	t.SetColumnConfigs(columnConfigs)
	t.AppendHeader(table.Row{"ID", "Dialer", "Acceptor", "Static Cost", "Src Latency", "Dst Latency", "State", "Status", "Full Cost"})

	for _, entity := range children {
		id := entity.Path("id").Data().(string)
		srcRouter := api.GetJsonString(entity, "sourceRouter.name")
		dstRouter := api.GetJsonString(entity, "destRouter.name")
		staticCost := entity.Path("staticCost").Data().(float64)
		srcLatency := entity.Path("sourceLatency").Data().(float64) / 1_000_000
		dstLatency := entity.Path("destLatency").Data().(float64) / 1_000_000
		state := api.GetJsonString(entity, "state")
		down := entity.Path("down").Data().(bool)
		cost := entity.Path("cost").Data().(float64)

		status := "up"
		if down {
			status = "down"
		}

		t.AppendRow(table.Row{id, srcRouter, dstRouter, staticCost,
			fmt.Sprintf("%.1fms", srcLatency),
			fmt.Sprintf("%.1fms", dstLatency),
			state, status, cost})
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
	t.AppendHeader(table.Row{"ID", "Service", "Router", "Binding", "Address", "Instance", "Cost", "Precedence", "Dynamic Cost", "Host ID"})

	for _, entity := range children {
		id := api.GetJsonString(entity, "id")
		service := api.GetJsonString(entity, "service.name")
		router := api.GetJsonString(entity, "router.name")
		binding := api.GetJsonString(entity, "binding")
		address := api.GetJsonString(entity, "address")
		instanceId := api.GetJsonString(entity, "instanceId")
		staticCost := api.GetJsonString(entity, "cost")
		precedence := api.GetJsonString(entity, "precedence")
		dynamicCost := api.GetJsonString(entity, "dynamicCost")
		hostId := api.GetJsonString(entity, "hostId")

		t.AppendRow(table.Row{id, service, router, binding, address, instanceId, staticCost, precedence, dynamicCost, hostId})
	}
	api.RenderTable(o, t, pagingInfo)
	return nil
}

func runListServices(o *api.Options) error {
	children, pagingInfo, err := listEntitiesWithOptions("services", o)
	if err != nil {
		return err
	}
	return outputServices(o, children, pagingInfo)
}

func outputServices(o *api.Options, children []*gabs.Container, pagingInfo *api.Paging) error {
	if o.OutputJSONResponse {
		return nil
	}

	t := table.NewWriter()
	t.SetStyle(table.StyleRounded)
	t.AppendHeader(table.Row{"ID", "Name", "Terminator Strategy"})

	for _, entity := range children {
		id := entity.Path("id").Data().(string)
		name := entity.Path("name").Data().(string)
		terminatorStrategy, _ := entity.Path("terminatorStrategy").Data().(string)
		t.AppendRow(table.Row{id, name, terminatorStrategy})
	}

	api.RenderTable(o, t, pagingInfo)

	return nil
}

func runListRouters(o *api.Options) error {
	children, pagingInfo, err := listEntitiesWithOptions("routers", o)
	if err != nil {
		return err
	}
	return outputRouters(o, children, pagingInfo)
}

func outputRouters(o *api.Options, children []*gabs.Container, pagingInfo *api.Paging) error {
	if o.OutputJSONResponse {
		return nil
	}

	t := table.NewWriter()
	t.SetStyle(table.StyleRounded)
	t.AppendHeader(table.Row{"ID", "Name", "Online", "Cost", "No Traversal", "Version", "Listeners"})

	for _, entity := range children {
		id := entity.Path("id").Data().(string)
		name := entity.Path("name").Data().(string)
		connected := entity.Path("connected").Data().(bool)
		cost := entity.Path("cost").Data().(float64)
		versionInfo := entity.Path("versionInfo")
		noTraversal := entity.Path("noTraversal").Data().(bool)
		var version string
		if versionInfo != nil {
			v := versionInfo.Path("version").Data().(string)
			os := versionInfo.Path("os").Data().(string)
			arch := versionInfo.Path("arch").Data().(string)
			version = fmt.Sprintf("%v on %v/%v", v, os, arch)
		}
		var listeners []string
		listenerAddresses := entity.Path("listenerAddresses")
		if listenerAddresses != nil {
			children, _ := listenerAddresses.Children()
			for idx, child := range children {
				addr := child.Path("address").Data().(string)
				listeners = append(listeners, fmt.Sprintf("%v: %v", idx+1, addr))
			}
		}
		t.AppendRow(table.Row{id, name, connected, cost, noTraversal, version, strings.Join(listeners, "\n")})
	}

	api.RenderTable(o, t, pagingInfo)

	return nil
}
