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
	"errors"
	"fmt"
	"github.com/openziti/edge-api/rest_management_api_client/service_policy"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	"github.com/openziti/ziti/internal/rest/mgmt"
)

func (u *Upload) ProcessServicePolicies(input map[string][]interface{}) (map[string]string, error) {

	var result = map[string]string{}
	for _, data := range input["servicePolicies"] {
		create := FromMap(data, rest_model.ServicePolicyCreate{})

		// see if the service policy already exists
		existing := mgmt.ServicePolicyFromFilter(u.client, mgmt.NameFilter(*create.Name))
		if existing != nil {
			log.WithFields(map[string]interface{}{
				"name":            *create.Name,
				"servicePolicyId": *existing.ID,
			}).
				Info("Found existing ServicePolicy, skipping create")
			_, _ = fmt.Fprintf(u.Err, "\u001B[2KSkipping ServicePolicy %s\r", *create.Name)
			continue
		}

		// look up the service ids from the name and add to the create
		serviceRoles, err := u.lookupServices(create.ServiceRoles)
		if err != nil {
			return nil, err
		}
		create.ServiceRoles = serviceRoles

		// look up the identity ids from the name and add to the create
		identityRoles, err := u.lookupIdentities(create.IdentityRoles)
		if err != nil {
			return nil, errors.Join(errors.New("Unable to read all identities from ServicePolicy"), err)
		}
		create.IdentityRoles = identityRoles

		// do the actual create since it doesn't exist
		_, _ = fmt.Fprintf(u.Err, "\u001B[2KSkipping ServicePolicy %s\r", *create.Name)
		if u.verbose {
			log.WithField("name", *create.Name).Debug("Creating ServicePolicy")
		}
		created, createErr := u.client.ServicePolicy.CreateServicePolicy(&service_policy.CreateServicePolicyParams{Policy: create}, nil)
		if createErr != nil {
			if payloadErr, ok := createErr.(rest_util.ApiErrorPayload); ok {
				log.WithFields(map[string]interface{}{
					"field":  payloadErr.GetPayload().Error.Cause.APIFieldError.Field,
					"reason": payloadErr.GetPayload().Error.Cause.APIFieldError.Reason,
				}).
					Error("Unable to create ServicePolicy")
			} else {
				log.WithError(createErr).Error("Unable to ")
				return nil, createErr
			}
		}
		if u.verbose {
			log.WithFields(map[string]interface{}{
				"name":            *create.Name,
				"servicePolicyId": created.Payload.Data.ID,
			}).
				Info("Created ServicePolicy")
		}

		result[*create.Name] = created.Payload.Data.ID
	}

	return result, nil
}
