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
	"fmt"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"io"

	"github.com/pkg/errors"

	"github.com/Jeffail/gabs"
	"github.com/spf13/cobra"
)

type updateServiceEdgeRouterPolicyOptions struct {
	api.Options
	name            string
	edgeRouterRoles []string
	serviceRoles    []string
	tags            map[string]string
}

func newUpdateServiceEdgeRouterPolicyCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &updateServiceEdgeRouterPolicyOptions{
		Options: api.Options{
			CommonOptions: common.CommonOptions{Out: out, Err: errOut},
		},
	}

	cmd := &cobra.Command{
		Use:     "service-edge-router-policy <idOrName>",
		Aliases: []string{"serp"},
		Short:   "updates a service edge router policy managed by the Ziti Edge Controller",
		Long:    "updates a service edge router policy managed by the Ziti Edge Controller",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runUpdateServiceEdgeRouterPolicy(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().StringVarP(&options.name, "name", "n", "", "Set the name of the edge router policy")
	cmd.Flags().StringSliceVar(&options.edgeRouterRoles, "edge-router-roles", nil, "Edge router roles of the service edge router policy")
	cmd.Flags().StringSliceVar(&options.serviceRoles, "service-roles", nil, "Service roles of the service edge router policy")
	cmd.Flags().StringToStringVar(&options.tags, "tags", nil, "Custom management tags")

	options.AddCommonFlags(cmd)

	return cmd
}

func runUpdateServiceEdgeRouterPolicy(o *updateServiceEdgeRouterPolicyOptions) error {
	id, err := mapNameToID("service-edge-router-policies", o.Args[0], o.Options)
	if err != nil {
		return err
	}

	edgeRouterRoles, err := convertNamesToIds(o.edgeRouterRoles, "edge-routers", o.Options)
	if err != nil {
		return err
	}

	serviceRoles, err := convertNamesToIds(o.serviceRoles, "services", o.Options)
	if err != nil {
		return err
	}

	entityData := gabs.New()
	change := false

	if o.Cmd.Flags().Changed("name") {
		api.SetJSONValue(entityData, o.name, "name")
		change = true
	}

	if o.Cmd.Flags().Changed("edge-router-roles") {
		api.SetJSONValue(entityData, edgeRouterRoles, "edgeRouterRoles")
		change = true
	}

	if o.Cmd.Flags().Changed("service-roles") {
		api.SetJSONValue(entityData, serviceRoles, "serviceRoles")
		change = true
	}

	if o.Cmd.Flags().Changed("tags") {
		api.SetJSONValue(entityData, o.tags, "tags")
		change = true
	}

	if !change {
		return errors.New("no change specified. must specify at least one attribute to change")
	}

	_, err = patchEntityOfType(fmt.Sprintf("service-edge-router-policies/%v", id), entityData.String(), &o.Options)
	return err
}
