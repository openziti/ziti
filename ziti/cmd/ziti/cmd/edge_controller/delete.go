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
	"io"
	"strings"

	"github.com/Jeffail/gabs"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/common"
	cmdutil "github.com/openziti/ziti/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/ziti/util"
	"github.com/spf13/cobra"
)

// newDeleteCmd creates a command object for the "edge controller delete" command
func newDeleteCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "deletes various entities managed by the Ziti Edge Controller",
		Long:  "deletes various entities managed by the Ziti Edge Controller",
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

	cmd.AddCommand(newDeleteCmdForEntityType("api-session", runDeleteEntityOfType, newOptions()))
	cmd.AddCommand(newDeleteAuthenticatorCmd("authenticator", newOptions()))
	cmd.AddCommand(newDeleteCmdForEntityType("ca", runDeleteEntityOfType, newOptions()))
	cmd.AddCommand(newDeleteCmdForEntityType("config", runDeleteEntityOfType, newOptions()))
	cmd.AddCommand(newDeleteCmdForEntityType("config-type", runDeleteEntityOfType, newOptions()))
	cmd.AddCommand(newDeleteCmdForEntityType("edge-router", runDeleteEntityOfType, newOptions()))
	cmd.AddCommand(newDeleteCmdForEntityType("edge-router-policy", runDeleteEntityOfType, newOptions()))
	cmd.AddCommand(newDeleteCmdForEntityType("identity", runDeleteEntityOfType, newOptions()))
	cmd.AddCommand(newDeleteCmdForEntityType("service", runDeleteEntityOfType, newOptions()))
	cmd.AddCommand(newDeleteCmdForEntityType("service-edge-router-policy", runDeleteEntityOfType, newOptions()))
	cmd.AddCommand(newDeleteCmdForEntityType("service-policy", runDeleteEntityOfType, newOptions()))
	cmd.AddCommand(newDeleteCmdForEntityType("session", runDeleteEntityOfType, newOptions()))
	cmd.AddCommand(newDeleteCmdForEntityType("terminator", runDeleteEntityOfType, newOptions()))
	cmd.AddCommand(newDeleteCmdForEntityType("posture-check", runDeleteEntityOfType, newOptions()))

	return cmd
}

type deleteCmdRunner func(*edgeOptions, string) error

// newDeleteCmdForEntityType creates the delete command for the given entity type
func newDeleteCmdForEntityType(entityType string, command deleteCmdRunner, options *edgeOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   entityType + " <id>",
		Short: "deletes entity of type " + entityType + " managed by the Ziti Edge Controller",
		Long:  "deletes entity of type " + entityType + " managed by the Ziti Edge Controller",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := command(options, getPlural(entityType))
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
func runDeleteEntityOfType(o *edgeOptions, entityType string) error {
	var err error
	ids := []string{o.Args[0]}
	if entityType != "terminators" && entityType != "api-sessions" && entityType != "sessions" {
		ids, err = mapNamesToIDs(entityType, *o, o.Args[0])
	}
	if err != nil {
		return err
	}
	for _, id := range ids {
		_, err := util.EdgeControllerDelete(entityType, id, o.Out, o.OutputJSONResponse, o.Timeout, o.Verbose)
		if err != nil {
			return err
		}
	}
	return nil
}

func getPlural(entityType string) string {
	if strings.HasSuffix(entityType, "y") {
		return strings.TrimSuffix(entityType, "y") + "ies"
	}
	return entityType + "s"
}

func deleteEntityOfType(entityType string, id string, options *edgeOptions) (*gabs.Container, error) {
	return util.EdgeControllerDelete(entityType, id, options.Out, options.OutputJSONResponse, options.Timeout, options.Verbose)
}
