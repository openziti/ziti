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

package upload

import (
	"fmt"
	"github.com/openziti/edge-api/rest_management_api_client/edge_router_policy"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	"github.com/openziti/ziti/internal/rest/mgmt"
)

func (u *Upload) ProcessEdgeRouterPolicies(input map[string][]interface{}) (map[string]string, error) {

	var result = map[string]string{}
	for _, data := range input["edgeRouterPolicies"] {
		create := FromMap(data, rest_model.EdgeRouterPolicyCreate{})

		// see if the router already exists
		existing := mgmt.EdgeRouterPolicyFromFilter(u.client, mgmt.NameFilter(*create.Name))
		if existing != nil {
			log.WithFields(map[string]interface{}{
				"name":               *create.Name,
				"edgeRouterPolicyId": *existing.ID,
			}).
				Info("Found existing EdgeRouterPolicy, skipping create")
			_, _ = fmt.Fprintf(u.Err, "\u001B[2KSkipping EdgeRouterPolicy %s\r", *create.Name)
			continue
		}

		// look up the edgeRouter ids from the name and add to the create
		edgeRouterRoles, err := u.lookupEdgeRouters(create.EdgeRouterRoles)
		if err != nil {
			return nil, err
		}
		create.EdgeRouterRoles = edgeRouterRoles

		// look up the identity ids from the name and add to the create
		identityRoles, err := u.lookupIdentities(create.IdentityRoles)
		if err != nil {
			return nil, err
		}
		create.IdentityRoles = identityRoles

		// do the actual create since it doesn't exist
		_, _ = fmt.Fprintf(u.Err, "\u001B[2KCreating EdgeRouterPolicy %s\r", *create.Name)
		if u.verbose {
			log.WithField("name", *create.Name).Debug("Creating EdgeRouterPolicy")
		}
		created, createErr := u.client.EdgeRouterPolicy.CreateEdgeRouterPolicy(&edge_router_policy.CreateEdgeRouterPolicyParams{Policy: create}, nil)
		if createErr != nil {
			if payloadErr, ok := createErr.(rest_util.ApiErrorPayload); ok {
				log.WithFields(map[string]interface{}{
					"field":  payloadErr.GetPayload().Error.Cause.APIFieldError.Field,
					"reason": payloadErr.GetPayload().Error.Cause.APIFieldError.Reason,
				}).
					Error("Unable to create EdgeRouterPolicy")
				return nil, createErr
			} else {
				log.WithError(createErr).Error("Unable to create EdgeRouterPolicy")
				return nil, createErr
			}
		}
		if u.verbose {
			log.WithFields(map[string]interface{}{
				"name":           *create.Name,
				"routerPolicyId": created.Payload.Data.ID,
			}).
				Info("Created EdgeRouterPolicy")
		}

		result[*create.Name] = created.Payload.Data.ID
	}

	return result, nil
}
