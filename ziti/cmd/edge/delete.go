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
	"github.com/fatih/color"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/util"
	"io"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// newDeleteCmd creates a command object for the "edge controller delete" command
func newDeleteCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "deletes various entities managed by the Ziti Edge Controller",
		Long:  "deletes various entities managed by the Ziti Edge Controller",
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			cmdhelper.CheckErr(err)
		},
	}

	newOptions := func() *api.Options {
		return &api.Options{
			CommonOptions: common.CommonOptions{
				Out: out,
				Err: errOut,
			},
		}
	}

	cmd.AddCommand(newDeleteCmdForEntityType("api-session", newOptions()))
	cmd.AddCommand(newDeleteCmdForEntityType("authenticator", newOptions()))
	cmd.AddCommand(newDeleteCmdForEntityType("enrollment", newOptions()))
	cmd.AddCommand(newDeleteCmdForEntityType("ca", newOptions()))
	cmd.AddCommand(newDeleteCmdForEntityType("config", newOptions()))
	cmd.AddCommand(newDeleteCmdForEntityType("config-type", newOptions()))
	cmd.AddCommand(newDeleteCmdForEntityType("edge-router", newOptions(), "er", "ers"))
	cmd.AddCommand(newDeleteCmdForEntityType("edge-router-policy", newOptions(), "erp", "erps"))
	cmd.AddCommand(newDeleteCmdForEntityType("identity", newOptions()))
	cmd.AddCommand(newDeleteCmdForEntityType("posture-check", newOptions()))
	cmd.AddCommand(newDeleteCmdForEntityType("service", newOptions()))
	cmd.AddCommand(newDeleteCmdForEntityType("service-edge-router-policy", newOptions(), "serp", "serps"))
	cmd.AddCommand(newDeleteCmdForEntityType("service-policy", newOptions(), "sp", "sps"))
	cmd.AddCommand(newDeleteCmdForEntityType("session", newOptions()))
	cmd.AddCommand(newDeleteCmdForEntityType("terminator", newOptions()))
	cmd.AddCommand(newDeleteCmdForEntityType("transit-router", newOptions()))

	return cmd
}

// newDeleteCmdForEntityType creates the delete command for the given entity type
func newDeleteCmdForEntityType(entityType string, options *api.Options, aliases ...string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     entityType + " <id>",
		Short:   "deletes " + getPlural(entityType) + " managed by the Ziti Edge Controller",
		Args:    cobra.MinimumNArgs(1),
		Aliases: append(aliases, getPlural(entityType)),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runDeleteEntityOfType(options, getPlural(entityType))
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	options.AddCommonFlags(cmd)

	cmd.AddCommand(newDeleteWhereCmdForEntityType(entityType, options))

	return cmd
}

func newDeleteWhereCmdForEntityType(entityType string, options *api.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "where <filter>",
		Short: "deletes " + getPlural(entityType) + " matching the filter managed by the Ziti Edge Controller",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runDeleteEntityOfTypeWhere(options, getPlural(entityType))
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	options.AddCommonFlags(cmd)

	return cmd
}

// runDeleteEntityOfType implements the commands to delete various entity types
func runDeleteEntityOfType(o *api.Options, entityType string) error {
	var err error
	ids := o.Args
	if entityType != "terminators" && entityType != "api-sessions" && entityType != "sessions" && entityType != "authenticators" && entityType != "enrollments" {
		ids, err = mapNamesToIDs(entityType, *o, ids...)
	}
	if err != nil {
		return err
	}
	return deleteEntitiesOfType(o, entityType, ids)
}

func deleteEntitiesOfType(o *api.Options, entityType string, ids []string) error {
	for _, id := range ids {
		err := util.ControllerDelete("edge", entityType, id, "", o.Out, o.OutputJSONRequest, o.OutputJSONResponse, o.Timeout, o.Verbose)
		if err != nil {
			o.Printf("delete of %v with id %v: %v\n", boltz.GetSingularEntityType(entityType), id, color.New(color.FgRed, color.Bold).Sprint("FAIL"))
			return err
		}
		o.Printf("delete of %v with id %v: %v\n", boltz.GetSingularEntityType(entityType), id, color.New(color.FgGreen, color.Bold).Sprint("OK"))
	}
	return nil
}

// runDeleteEntityOfType implements the commands to delete various entity types
func runDeleteEntityOfTypeWhere(options *api.Options, entityType string) error {
	filter := strings.Join(options.Args, " ")

	params := url.Values{}
	params.Add("filter", filter)

	children, pageInfo, err := ListEntitiesOfType(entityType, params, options.OutputJSONResponse, options.Out, options.Timeout, options.Verbose)
	if err != nil {
		return err
	}

	options.Printf("filter returned ")
	pageInfo.Output(options)

	var ids []string
	for _, entity := range children {
		id, _ := entity.Path("id").Data().(string)
		ids = append(ids, id)
	}

	return deleteEntitiesOfType(options, entityType, ids)
}

func getPlural(entityType string) string {
	if strings.HasSuffix(entityType, "y") {
		return strings.TrimSuffix(entityType, "y") + "ies"
	}
	return entityType + "s"
}

func deleteEntityOfType(entityType string, id string, options *api.Options) error {
	return util.ControllerDelete("edge", entityType, id, "", options.Out, options.OutputJSONRequest, options.OutputJSONResponse, options.Timeout, options.Verbose)
}
