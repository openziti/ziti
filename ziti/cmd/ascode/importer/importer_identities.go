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
	"strings"
)

func (importer *Importer) IsIdentityImportRequired(args []string) bool {
	return slices.Contains(args, "all") || len(args) == 0 || // explicit all or nothing specified
		slices.Contains(args, "identity")
}

func (importer *Importer) ProcessIdentities(input map[string][]interface{}) (map[string]string, map[string]string, error) {

	var createdResult = map[string]string{}
	var updatedResult = map[string]string{}

	for _, data := range input["identities"] {

		// convert to a json doc so we can query inside the data
		jsonData, _ := json.Marshal(data)
		doc, jsonParseError := gabs.ParseJSON(jsonData)
		if jsonParseError != nil {
			log.WithError(jsonParseError).Error("Unable to parse json")
			return nil, nil, jsonParseError
		}

		name := doc.Path("name").Data().(string)
		existing := mgmt.IdentityFromFilter(importer.Client, mgmt.NameFilter(name))
		if existing != nil {
			log.WithFields(map[string]interface{}{
				"name":       name,
				"identityId": *existing.ID,
			}).
				Info("Found existing Identity, skipping create")
			_, _ = internal.FPrintfReusingLine(importer.Err, "Skipping Identity %s\r", name)

			// if the identity exists, and it's not a 'default' identity or it's the default admin, update the identity's attributes from the input data
			if strings.ToLower(*existing.TypeID) != "default" || *existing.IsDefaultAdmin {

				var attributes = rest_model.Attributes{}
				for _, attr := range doc.Path("roleAttributes").Data().([]interface{}) {
					attributes = append(attributes, ""+attr.(string)+"")
				}
				update := rest_model.IdentityPatch{
					RoleAttributes: &attributes,
				}
				_, _ = internal.FPrintfReusingLine(importer.Err, "Updating Identity %s's attributes\r", name)
				_, updateErr := importer.Client.Identity.PatchIdentity(&identity.PatchIdentityParams{ID: *existing.ID, Identity: &update}, nil)
				if updateErr != nil {
					if payloadErr, ok := updateErr.(rest_util.ApiErrorPayload); ok {
						log.WithFields(map[string]interface{}{
							"field":  payloadErr.GetPayload().Error.Cause.APIFieldError.Field,
							"reason": payloadErr.GetPayload().Error.Cause.APIFieldError.Reason,
						}).
							Error("Unable to update Identity")
						return nil, nil, updateErr
					} else {
						log.WithError(updateErr).Error("Unable to update Identity")
						return nil, nil, updateErr
					}
				}

				log.WithFields(map[string]interface{}{
					"name":       name,
					"identityId": *existing.ID,
				}).
					Info("Updated identity")

				updatedResult[name] = *existing.ID
			}
			continue
		}

		create := FromMap(data, rest_model.IdentityCreate{})
		if "true" == doc.Path("isDefaultAdmin").Data() {
			log.Debug("Not creating default admin")
			continue
		}

		typeId := doc.Path("TypeID").Data()
		if typeId == nil || strings.ToLower(typeId.(string)) == "default" {
			create.Type = rest_model.IdentityType.Pointer(rest_model.IdentityTypeDefault)
		} else if strings.ToLower(typeId.(string)) == "router" {
			create.Type = rest_model.IdentityType.Pointer(rest_model.IdentityTypeRouter)
		}

		policyName := doc.Path("authPolicy").Data().(string)[1:]

		// look up the auth policy id from the name and add to the create, omit if it's the "Default" policy
		policy, _ := ascode.GetItemFromCache(importer.authPolicyCache, policyName, func(name string) (interface{}, error) {
			return mgmt.AuthPolicyFromFilter(importer.Client, mgmt.NameFilter(name)), nil
		})
		if policy == nil {
			return nil, nil, errors.New("error reading Auth Policy: " + policyName)
		}
		if policy != "" && policy != "Default" {
			create.AuthPolicyID = policy.(*rest_model.AuthPolicyDetail).ID
		}

		// do the actual creation since it doesn't exist, but only if it's a "default" identity
		if *create.Type == rest_model.IdentityTypeDefault {
			_, _ = internal.FPrintfReusingLine(importer.Err, "Creating Identity %s\r", *create.Name)
			created, createErr := importer.Client.Identity.CreateIdentity(&identity.CreateIdentityParams{Identity: create}, nil)
			if createErr != nil {
				if payloadErr, ok := createErr.(rest_util.ApiErrorPayload); ok {
					log.WithFields(map[string]interface{}{
						"field":  payloadErr.GetPayload().Error.Cause.APIFieldError.Field,
						"reason": payloadErr.GetPayload().Error.Cause.APIFieldError.Reason,
					}).
						Error("Unable to create Identity")
					return nil, nil, createErr
				} else {
					log.WithError(createErr).Error("Unable to create Identity")
					return nil, nil, createErr
				}
			}
			log.WithFields(map[string]interface{}{
				"name":       *create.Name,
				"identityId": created.Payload.Data.ID,
			}).
				Info("Created identity")

			createdResult[*create.Name] = created.Payload.Data.ID
		}

	}

	return createdResult, updatedResult, nil
}

func (importer *Importer) lookupIdentities(roles []string) ([]string, error) {
	identityRoles := []string{}
	for _, role := range roles {
		if role[0:1] == "@" {
			roleName := role[1:]
			value, lookupErr := ascode.GetItemFromCache(importer.identityCache, roleName, func(name string) (interface{}, error) {
				return mgmt.IdentityFromFilter(importer.Client, mgmt.NameFilter(name)), nil
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
