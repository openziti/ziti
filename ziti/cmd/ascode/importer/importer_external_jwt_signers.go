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
	"github.com/openziti/edge-api/rest_management_api_client"
	"github.com/openziti/edge-api/rest_management_api_client/external_jwt_signer"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	"github.com/openziti/ziti/internal"
	"github.com/openziti/ziti/internal/rest/mgmt"
	"slices"
)

func (importer *Importer) IsExtJwtSignerImportRequired(args []string) bool {
	return slices.Contains(args, "all") || len(args) == 0 || // explicit all or nothing specified
		slices.Contains(args, "ext-jwt-signer") ||
		slices.Contains(args, "external-jwt-signer") ||
		slices.Contains(args, "auth-policy") ||
		slices.Contains(args, "identity")
}

func (importer *Importer) ProcessExternalJwtSigners(client *rest_management_api_client.ZitiEdgeManagement, input map[string][]interface{}) (map[string]string, error) {

	var result = map[string]string{}
	for _, data := range input["externalJwtSigners"] {
		create := FromMap(data, rest_model.ExternalJWTSignerCreate{})

		// see if the signer already exists
		existing := mgmt.ExternalJWTSignerFromFilter(client, mgmt.NameFilter(*create.Name))
		if existing != nil {
			log.WithFields(map[string]interface{}{
				"name":                *create.Name,
				"externalJwtSignerId": *existing.ID,
			}).
				Info("Found existing ExtJWTSigner, skipping create")
			_, _ = internal.FPrintfReusingLine(importer.Err, "Skipping ExtJWTSigner %s\r", *create.Name)
			continue
		}

		// do the actual create since it doesn't exist
		_, _ = internal.FPrintfReusingLine(importer.Err, "Creating ExtJWTSigner %s\r", *create.Name)
		log.WithField("name", *create.Name).Debug("Creating ExtJWTSigner")
		created, createErr := client.ExternalJWTSigner.CreateExternalJWTSigner(&external_jwt_signer.CreateExternalJWTSignerParams{ExternalJWTSigner: create}, nil)
		if createErr != nil {
			if payloadErr, ok := createErr.(rest_util.ApiErrorPayload); ok {
				log.WithFields(map[string]interface{}{
					"field":  payloadErr.GetPayload().Error.Cause.APIFieldError.Field,
					"reason": payloadErr.GetPayload().Error.Cause.APIFieldError.Reason,
					"err":    payloadErr,
				}).
					Error("Unable to create ExtJWTSigner")
				return nil, createErr
			} else {
				log.Error("Unable to create ExtJWTSigner")
				return nil, createErr
			}
		}
		log.WithFields(map[string]interface{}{
			"name":                *create.Name,
			"externalJwtSignerId": created.Payload.Data.ID,
		}).
			Info("Created ExtJWTSigner")

		result[*create.Name] = created.Payload.Data.ID
	}

	return result, nil
}
