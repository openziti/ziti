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
	"fmt"
	"github.com/antchfx/jsonquery"
	"github.com/openziti/edge-api/rest_management_api_client/auth_policy"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	common "github.com/openziti/ziti/internal/ascode"
	"github.com/openziti/ziti/internal/rest/mgmt"
	"strings"
)

func (u *Upload) ProcessAuthPolicies(input map[string][]interface{}) (map[string]string, error) {

	if u.verbose {
		log.Debug("Listing all AuthPolicies")
	}

	result := map[string]string{}

	for _, data := range input["authPolicies"] {
		create := FromMap(data, rest_model.AuthPolicyCreate{})

		// see if the auth policy already exists
		existing := mgmt.AuthPolicyFromFilter(u.client, mgmt.NameFilter(*create.Name))
		if existing != nil {
			if u.verbose {
				log.WithFields(map[string]interface{}{
					"name":         *create.Name,
					"authPolicyId": *existing.ID,
				}).Info("Found existing Auth Policy, skipping create")
			}
			_, _ = fmt.Fprintf(u.Err, "\u001B[2KSkipping AuthPolicy %s\r", *create.Name)
			continue
		}

		// convert to a jsonquery doc so we can query inside the json
		jsonData, _ := json.Marshal(data)
		doc, jsonQueryErr := jsonquery.Parse(strings.NewReader(string(jsonData)))
		if jsonQueryErr != nil {
			log.WithError(jsonQueryErr).Error("Unable to list AuthPolicies")
			return nil, jsonQueryErr
		}
		allowedSigners := jsonquery.FindOne(doc, "/primary/extJwt/allowedSigners")

		// look up each signer by name and add to the create
		allowedSignerIds := []string{}
		for _, signer := range allowedSigners.ChildNodes() {
			value := signer.Value().(string)[1:]
			extJwtSigner, err := common.GetItemFromCache(u.extJwtSignersCache, value, func(name string) (interface{}, error) {
				return mgmt.ExternalJWTSignerFromFilter(u.client, mgmt.NameFilter(name)), nil
			})
			if err != nil {
				log.WithField("name", *create.Name).Warn("Unable to read ExtJwtSigner")
				return nil, err
			}
			allowedSignerIds = append(allowedSignerIds, *extJwtSigner.(*rest_model.ExternalJWTSignerDetail).ID)
		}
		create.Primary.ExtJWT.AllowedSigners = allowedSignerIds

		// do the actual create since it doesn't exist
		_, _ = fmt.Fprintf(u.Err, "\u001B[2KCreating AuthPolicy %s\r", *create.Name)
		if u.verbose {
			log.WithField("name", *create.Name).
				Debug("Creating AuthPolicy")
		}
		created, createErr := u.client.AuthPolicy.CreateAuthPolicy(&auth_policy.CreateAuthPolicyParams{AuthPolicy: create}, nil)
		if createErr != nil {
			if payloadErr, ok := createErr.(rest_util.ApiErrorPayload); ok {
				log.WithFields(map[string]interface{}{
					"field":  payloadErr.GetPayload().Error.Cause.APIFieldError.Field,
					"reason": payloadErr.GetPayload().Error.Cause.APIFieldError.Reason,
					"err":    payloadErr,
				}).Error("Unable to create AuthPolicy")
				return nil, createErr
			} else {
				log.WithError(createErr).Error("Unable to create AuthPolicy")
				return nil, createErr
			}
		}

		if u.verbose {
			log.WithFields(map[string]interface{}{
				"name":         *create.Name,
				"authPolicyId": created.Payload.Data.ID,
			}).Info("Created AuthPolicy")
		}

		result[*create.Name] = created.Payload.Data.ID
	}

	return result, nil
}
