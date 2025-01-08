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

package importer

import (
	"encoding/json"
	"errors"
	"github.com/Jeffail/gabs/v2"
	"github.com/openziti/edge-api/rest_management_api_client/identity"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	"github.com/openziti/ziti/internal"
	"github.com/openziti/ziti/internal/ascode"
	"github.com/openziti/ziti/internal/rest/mgmt"
	"slices"
)

func (u *Importer) IsIdentityImportRequired(args []string) bool {
	return slices.Contains(args, "all") || len(args) == 0 || // explicit all or nothing specified
		slices.Contains(args, "identity")
}

func (u *Importer) ProcessIdentities(input map[string][]interface{}) (map[string]string, error) {

	var result = map[string]string{}

	for _, data := range input["identities"] {
		create := FromMap(data, rest_model.IdentityCreate{})

		existing := mgmt.IdentityFromFilter(u.client, mgmt.NameFilter(*create.Name))
		if existing != nil {
			if u.loginOpts.Verbose {
				log.WithFields(map[string]interface{}{
					"name":       *create.Name,
					"identityId": *existing.ID,
				}).
					Info("Found existing Identity, skipping create")
			}
			_, _ = internal.FPrintfReusingLine(u.loginOpts.Err, "Skipping Identity %s\r", *create.Name)
			continue
		}

		// set the type because it is not in the input
		typ := rest_model.IdentityTypeDefault
		create.Type = &typ

		// convert to a json doc so we can query inside the data
		jsonData, _ := json.Marshal(data)
		doc, jsonParseError := gabs.ParseJSON(jsonData)
		if jsonParseError != nil {
			log.WithError(jsonParseError).Error("Unable to parse json")
			return nil, jsonParseError
		}
		policyName := doc.Path("authPolicy").Data().(string)[1:]

		// look up the auth policy id from the name and add to the create, omit if it's the "Default" policy
		policy, _ := ascode.GetItemFromCache(u.authPolicyCache, policyName, func(name string) (interface{}, error) {
			return mgmt.AuthPolicyFromFilter(u.client, mgmt.NameFilter(name)), nil
		})
		if policy == nil {
			return nil, errors.New("error reading Auth Policy: " + policyName)
		}
		if policy != "" && policy != "Default" {
			create.AuthPolicyID = policy.(*rest_model.AuthPolicyDetail).ID
		}

		// do the actual create since it doesn't exist
		_, _ = internal.FPrintfReusingLine(u.loginOpts.Err, "Creating Identity %s\r", *create.Name)
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
		if u.loginOpts.Verbose {
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

func (u *Importer) lookupIdentities(roles []string) ([]string, error) {
	identityRoles := []string{}
	for _, role := range roles {
		if role[0:1] == "@" {
			roleName := role[1:]
			value, lookupErr := ascode.GetItemFromCache(u.identityCache, roleName, func(name string) (interface{}, error) {
				return mgmt.IdentityFromFilter(u.client, mgmt.NameFilter(name)), nil
			})
			if lookupErr != nil {
				return nil, lookupErr
			}
			ident := value.(*rest_model.IdentityDetail)
			if ident == nil {
				return nil, errors.New("error reading Identity: " + roleName)
			}
			identityRoles = append(identityRoles, "@"+*ident.ID)
		} else {
			identityRoles = append(identityRoles, role)
		}
	}
	return identityRoles, nil
}
