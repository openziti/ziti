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
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"io"

	"github.com/Jeffail/gabs"
	"github.com/spf13/cobra"
)

type createServiceEdgeRouterPolicyOptions struct {
	api.Options
	edgeRouterRoles []string
	serviceRoles    []string
	semantic        string
}

// newCreateServiceEdgeRouterPolicyCmd creates the 'edge controller create service-edge-router-policy' command
func newCreateServiceEdgeRouterPolicyCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &createServiceEdgeRouterPolicyOptions{
		Options: api.Options{
			CommonOptions: common.CommonOptions{Out: out, Err: errOut},
		},
	}

	cmd := &cobra.Command{
		Use:     "service-edge-router-policy <name>",
		Aliases: []string{"serp"},
		Short:   "creates a service-edge-router-policy managed by the Ziti Edge Controller",
		Long:    "creates a service-edge-router-policy managed by the Ziti Edge Controller",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runCreateServiceEdgeRouterPolicy(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().StringSliceVar(&options.edgeRouterRoles, "edge-router-roles", nil, "Edge router roles of the new service edge router policy")
	cmd.Flags().StringSliceVar(&options.serviceRoles, "service-roles", nil, "Identity roles of the new service edge router policy")
	cmd.Flags().StringVar(&options.semantic, "semantic", "AllOf", "Semantic dictating how multiple attributes should be interpreted. Valid values: AnyOf, AllOf")
	options.AddCommonFlags(cmd)

	return cmd
}

// runCreateServiceEdgeRouterPolicy create a new edgeRouterPolicy on the Ziti Edge Controller
func runCreateServiceEdgeRouterPolicy(o *createServiceEdgeRouterPolicyOptions) error {
	edgeRouterRoles, err := convertNamesToIds(o.edgeRouterRoles, "edge-routers", o.Options)
	if err != nil {
		return err
	}

	serviceRoles, err := convertNamesToIds(o.serviceRoles, "services", o.Options)
	if err != nil {
		return err
	}
	entityData := gabs.New()
	api.SetJSONValue(entityData, o.Args[0], "name")
	api.SetJSONValue(entityData, edgeRouterRoles, "edgeRouterRoles")
	api.SetJSONValue(entityData, serviceRoles, "serviceRoles")
	if o.semantic != "" {
		api.SetJSONValue(entityData, o.semantic, "semantic")
	}

	result, err := CreateEntityOfType("service-edge-router-policies", entityData.String(), &o.Options)
	return o.LogCreateResult("service edge router policy", result, err)
}
