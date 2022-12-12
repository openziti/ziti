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
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/util"
	"github.com/spf13/cobra"
)

type deleteOptions struct {
	api.Options
	isCircuit bool
	immediate bool
}

func (self *deleteOptions) AddCommonFlags(cmd *cobra.Command) {
	self.Options.AddCommonFlags(cmd)
	if self.isCircuit {
		if self.isCircuit {
			cmd.Flags().BoolVar(&self.immediate, "immediate", false, "delete the circuit immediately, without allowing a graceful shutdown")
		}
	}
}

func (self *deleteOptions) GetBody() string {
	if self.isCircuit {
		c := gabs.New()
		api.SetJSONValue(c, self.immediate, "immediate")
		return c.String()
	}
	return ""
}

// newDeleteCmd creates a command object for the "edge controller delete" command
func newDeleteCmd(p common.OptionsProvider) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "deletes various entities managed by the Ziti Controller",
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			cmdhelper.CheckErr(err)
		},
	}

	newOptions := func(isCircuit bool) *deleteOptions {
		return &deleteOptions{
			Options:   api.Options{CommonOptions: p()},
			isCircuit: true,
		}
	}

	cmd.AddCommand(newDeleteCmdForEntityType("circuit", newOptions(true)))
	cmd.AddCommand(newDeleteCmdForEntityType("link", newOptions(false)))
	cmd.AddCommand(newDeleteCmdForEntityType("router", newOptions(false)))
	cmd.AddCommand(newDeleteCmdForEntityType("service", newOptions(false)))
	cmd.AddCommand(newDeleteCmdForEntityType("terminator", newOptions(false)))

	return cmd
}

// newDeleteCmdForEntityType creates the delete command for the given entity type
func newDeleteCmdForEntityType(entityType string, options *deleteOptions, aliases ...string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     entityType + " <id>",
		Short:   "deletes " + api.GetPlural(entityType) + " managed by the Ziti Controller",
		Args:    cobra.MinimumNArgs(1),
		Aliases: append(aliases, api.GetPlural(entityType)),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runDeleteEntityOfType(options, api.GetPlural(entityType))
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

func newDeleteWhereCmdForEntityType(entityType string, options *deleteOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "where <filter>",
		Short: "deletes " + api.GetPlural(entityType) + " matching the filter managed by the Ziti Controller",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := api.DeleteEntityOfTypeWhere("fabric", &options.Options, api.GetPlural(entityType), options.GetBody())
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
func runDeleteEntityOfType(o *deleteOptions, entityType string) error {
	var err error
	ids := o.Args
	if entityType != "terminators" && entityType != "links" && entityType != "circuits" {
		ids, err = api.MapNamesToIDs("fabric", entityType, &o.Options, ids...)
	}
	if err != nil {
		return err
	}

	return api.DeleteEntitiesOfType(util.FabricAPI, &o.Options, entityType, ids, o.GetBody())
}
