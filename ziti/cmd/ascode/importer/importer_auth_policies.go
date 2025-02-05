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
	"github.com/Jeffail/gabs/v2"
	"github.com/openziti/edge-api/rest_management_api_client/auth_policy"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	"github.com/openziti/ziti/internal"
	"github.com/openziti/ziti/internal/ascode"
	"github.com/openziti/ziti/internal/rest/mgmt"
	"slices"
)

func (importer *Importer) IsAuthPolicyImportRequired(args []string) bool {
	return slices.Contains(args, "all") || len(args) == 0 || // explicit all or nothing specified
		slices.Contains(args, "auth-policy") ||
		slices.Contains(args, "identity")
}

func (importer *Importer) ProcessAuthPolicies(input map[string][]interface{}) (map[string]string, error) {

	result := map[string]string{}

	for _, data := range input["authPolicies"] {
		create := FromMap(data, rest_model.AuthPolicyCreate{})

		// see if the auth policy already exists
		existing := mgmt.AuthPolicyFromFilter(importer.Client, mgmt.NameFilter(*create.Name))
		if existing != nil {
			log.WithFields(map[string]interface{}{
				"name":         *create.Name,
				"authPolicyId": *existing.ID,
			}).Info("Found existing Auth Policy, skipping create")
			_, _ = internal.FPrintfReusingLine(importer.Err, "Skipping AuthPolicy %s\r", *create.Name)
			continue
		}

		// convert to a json doc so we can query inside the data
		jsonData, _ := json.Marshal(data)
		doc, jsonParseError := gabs.ParseJSON(jsonData)
		if jsonParseError != nil {
			log.WithError(jsonParseError).Error("Unable to parse json")
			return nil, jsonParseError
		}
		allowedSigners := doc.Path("primary.extJwt.allowedSigners")

		// look up each signer by name and add to the create
		allowedSignerIds := []string{}
		for _, signer := range allowedSigners.Children() {
			value := signer.Data().(string)[1:]
			extJwtSigner, err := ascode.GetItemFromCache(importer.extJwtSignersCache, value, func(name string) (interface{}, error) {
				return mgmt.ExternalJWTSignerFromFilter(importer.Client, mgmt.NameFilter(name)), nil
			})
			if err != nil {
				log.WithField("name", *create.Name).Warn("Unable to read ExtJwtSigner")
				return nil, err
			}
			allowedSignerIds = append(allowedSignerIds, *extJwtSigner.(*rest_model.ExternalJWTSignerDetail).ID)
		}
		create.Primary.ExtJWT.AllowedSigners = allowedSignerIds

		secondarySigner := doc.Path("secondary.requireExtJwtSigner")
		if secondarySigner.Data() != nil {

			// look up secondary signer by name and add to the create
			extJwtSigner, err := ascode.GetItemFromCache(importer.extJwtSignersCache, secondarySigner.Data().(string)[1:], func(name string) (interface{}, error) {
				return mgmt.ExternalJWTSignerFromFilter(importer.Client, mgmt.NameFilter(name)), nil
			})
			if err != nil {
				log.WithField("name", *create.Name).Warn("Unable to read ExtJwtSigner")
				return nil, err
			}
			create.Secondary.RequireExtJWTSigner = extJwtSigner.(*rest_model.ExternalJWTSignerDetail).ID
		}

		// do the actual create since it doesn't exist
		_, _ = internal.FPrintfReusingLine(importer.Err, "Creating AuthPolicy %s\r", *create.Name)
		log.WithField("name", *create.Name).
			Debug("Creating AuthPolicy")
		created, createErr := importer.Client.AuthPolicy.CreateAuthPolicy(&auth_policy.CreateAuthPolicyParams{AuthPolicy: create}, nil)
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

		log.WithFields(map[string]interface{}{
			"name":         *create.Name,
			"authPolicyId": created.Payload.Data.ID,
		}).Info("Created AuthPolicy")

		result[*create.Name] = created.Payload.Data.ID
	}

	return result, nil
}
