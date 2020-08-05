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
	"fmt"
	"io"
	"strings"

	"github.com/pkg/errors"

	"github.com/Jeffail/gabs"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/common"
	cmdutil "github.com/openziti/ziti/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/spf13/cobra"
)

type createServicePolicyOptions struct {
	commonOptions
	serviceRoles  []string
	identityRoles []string
}

// newCreateServicePolicyCmd creates the 'edge controller create service-policy' command
func newCreateServicePolicyCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &createServicePolicyOptions{
		commonOptions: commonOptions{
			CommonOptions: common.CommonOptions{Factory: f, Out: out, Err: errOut},
		},
	}

	cmd := &cobra.Command{
		Use:   "service-policy <name> <type>",
		Short: "creates a service-policy managed by the Ziti Edge Controller",
		Long:  "creates a service-policy managed by the Ziti Edge Controller",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runCreateServicePolicy(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().StringSliceVarP(&options.serviceRoles, "service-roles", "s", nil, "Service roles of the new service policy")
	cmd.Flags().StringSliceVarP(&options.identityRoles, "identity-roles", "i", nil, "Identity roles of the new service policy")
	options.AddCommonFlags(cmd)

	return cmd
}

// runCreateServicePolicy create a new servicePolicy on the Ziti Edge Controller
func runCreateServicePolicy(o *createServicePolicyOptions) error {
	policyType := o.Args[1]
	if policyType != "Bind" && policyType != "Dial" {
		return errors.Errorf("Invalid policy type '%v'. Valid values: [Bind, Dial]", policyType)
	}

	serviceRoles, err := convertNamesToIds(o.serviceRoles, "services")
	if err != nil {
		return err
	}

	identityRoles, err := convertNamesToIds(o.identityRoles, "identities")
	if err != nil {
		return err
	}

	entityData := gabs.New()
	setJSONValue(entityData, o.Args[0], "name")
	setJSONValue(entityData, o.Args[1], "type")
	setJSONValue(entityData, serviceRoles, "serviceRoles")
	setJSONValue(entityData, identityRoles, "identityRoles")
	result, err := createEntityOfType("service-policies", entityData.String(), &o.commonOptions)

	if err != nil {
		panic(err)
	}

	servicePolicyId := result.S("data", "id").Data()

	if _, err := fmt.Fprintf(o.Out, "%v\n", servicePolicyId); err != nil {
		panic(err)
	}

	return err
}

func convertNamesToIds(roles []string, entityType string) ([]string, error) {
	var result []string
	for _, val := range roles {
		if strings.HasPrefix(val, "@") {
			idOrName := strings.TrimPrefix(val, "@")
			id, err := mapNameToID(entityType, idOrName)
			if err != nil {
				return nil, err
			}
			result = append(result, "@"+id)
		} else {
			result = append(result, val)
		}
	}
	return result, nil
}
