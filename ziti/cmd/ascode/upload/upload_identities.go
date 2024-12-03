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
	"encoding/json"
	"errors"
	"github.com/antchfx/jsonquery"
	"github.com/openziti/edge-api/rest_management_api_client/identity"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	common "github.com/openziti/ziti/internal/ascode"
	"github.com/openziti/ziti/internal/rest/mgmt"
	"strings"
)

func (u *Upload) ProcessIdentities(input map[string][]interface{}) (map[string]string, error) {

	var result = map[string]string{}

	for _, data := range input["identities"] {
		create := FromMap(data, rest_model.IdentityCreate{})

		existing := mgmt.IdentityFromFilter(u.client, mgmt.NameFilter(*create.Name))
		if existing != nil {
			if u.verbose {
				log.WithFields(map[string]interface{}{
					"name":       *create.Name,
					"identityId": *existing.ID,
				}).
					Info("Found existing Identity, skipping create")
			}
			continue
		}

		// set the type because it is not in the input
		typ := rest_model.IdentityTypeDefault
		create.Type = &typ

		// convert to a jsonquery doc so we can query inside the json
		jsonData, _ := json.Marshal(data)
		doc, jsonQueryErr := jsonquery.Parse(strings.NewReader(string(jsonData)))
		if jsonQueryErr != nil {
			log.WithError(jsonQueryErr).
				Error("Unable to list Identities")
			return nil, jsonQueryErr
		}
		policyName := jsonquery.FindOne(doc, "/authPolicy").Value().(string)[1:]

		// look up the auth policy id from the name and add to the create, omit if it's the "Default" policy
		policy, _ := common.GetItemFromCache(u.authPolicyCache, policyName, func(name string) (interface{}, error) {
			return mgmt.AuthPolicyFromFilter(u.client, mgmt.NameFilter(name)), nil
		})
		if policy == nil {
			return nil, errors.New("error reading Auth Policy: " + policyName)
		}
		if policy != "" && policy != "Default" {
			create.AuthPolicyID = policy.(*rest_model.AuthPolicyDetail).ID
		}

		// do the actual create since it doesn't exist
		created, createErr := u.client.Identity.CreateIdentity(&identity.CreateIdentityParams{Identity: create}, nil)
		if createErr != nil {
			if payloadErr, ok := createErr.(rest_util.ApiErrorPayload); ok {
				log.WithFields(map[string]interface{}{
					"field":  payloadErr.GetPayload().Error.Cause.APIFieldError.Field,
					"reason": payloadErr.GetPayload().Error.Cause.APIFieldError.Reason,
				}).
					Error("Unable to create Identity")
				return nil, createErr
			} else {
				log.WithError(createErr).Error("Unable to create Identity")
				return nil, createErr
			}
		}
		if u.verbose {
			log.WithFields(map[string]interface{}{
				"name":       *create.Name,
				"identityId": created.Payload.Data.ID,
			}).
				Info("Created identity")
		}

		result[*create.Name] = created.Payload.Data.ID
	}

	return result, nil
}

func (u *Upload) lookupIdentities(roles []string) ([]string, error) {
	identityRoles := []string{}
	for _, role := range roles {
		if role[0:1] == "@" {
			value := role[1:]
			identity, _ := common.GetItemFromCache(u.identityCache, value, func(name string) (interface{}, error) {
				return mgmt.IdentityFromFilter(u.client, mgmt.NameFilter(name)), nil
			})
			if identity == nil {
				return nil, errors.New("error reading Identity: " + value)
			}
			identityId := identity.(*rest_model.IdentityDetail).ID
			identityRoles = append(identityRoles, "@"+*identityId)
		} else {
			identityRoles = append(identityRoles, role)
		}
	}
	return identityRoles, nil
}
