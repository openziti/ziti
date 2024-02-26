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
	"github.com/openziti/edge-api/rest_management_api_client/service"
	"github.com/openziti/ziti/controller/rest_client/terminator"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/openziti/ziti/ziti/util"
	"github.com/spf13/cobra"
)

type validateServiceHostingAction struct {
	api.Options
	filter string
}

func NewValidateServiceHostingCmd(p common.OptionsProvider) *cobra.Command {
	action := validateServiceHostingAction{
		Options: api.Options{
			CommonOptions: p(),
		},
	}

	validateServiceHostingCmd := &cobra.Command{
		Use:     "service-hosting",
		Short:   "Validate service hosting by comparing what is allowed to host services with what actually is hosting",
		Example: "ziti fabric validate service-hosting --filter 'name=\"test\"' --show-only-invalid",
		Args:    cobra.ExactArgs(0),
		RunE:    action.validateServiceHosting,
	}

	action.AddCommonFlags(validateServiceHostingCmd)
	validateServiceHostingCmd.Flags().StringVar(&action.filter, "filter", "sort by name limit none", "Specify which services to validate")
	return validateServiceHostingCmd
}

func (self *validateServiceHostingAction) validateServiceHosting(cmd *cobra.Command, _ []string) error {
	client, err := util.NewEdgeManagementClient(&self.Options)

	if err != nil {
		return err
	}

	fabricClient, err := util.NewFabricManagementClient(&self.Options)
	if err != nil {
		return err
	}

	context, cancelContext := self.Options.TimeoutContext()
	defer cancelContext()

	result, err := client.Service.ListServices(&service.ListServicesParams{
		Filter:  &self.filter,
		Context: context,
	}, nil)

	if err != nil {
		return util.WrapIfApiError(err)
	}

	policyType := "bind"

	identityFilter := "limit none"
	for _, svc := range result.Payload.Data {
		identitiesResult, err := client.Service.ListServiceIdentities(&service.ListServiceIdentitiesParams{
			ID:         *svc.ID,
			PolicyType: &policyType,
			Filter:     &identityFilter,
			Context:    context,
		}, nil)
		if err != nil {
			return err
		}

		identities := identitiesResult.Payload.Data
		if len(identities) == 0 {
			fmt.Printf("service '%s' is not hostable by any identities\n", *svc.Name)
		}

		for _, identity := range identities {
			filter := fmt.Sprintf(`service="%s" and hostId="%s" limit 1`, *svc.ID, *identity.ID)
			terminatorResult, err := fabricClient.Terminator.ListTerminators(&terminator.ListTerminatorsParams{
				Filter:  &filter,
				Limit:   nil,
				Offset:  nil,
				Context: context,
			})
			if err != nil {
				return err
			}
			fmt.Printf("service %s (%s) hosted by %s (%s) with %d terminators\n",
				*svc.Name, *svc.ID,
				*identity.Name, *identity.ID,
				*terminatorResult.Payload.Meta.Pagination.TotalCount)
		}
	}

	return nil
}
