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
	"github.com/openziti/edge-api/rest_management_api_client/external_jwt_signer"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	"github.com/openziti/ziti/internal/rest/mgmt"
)

func (u *Upload) ProcessExternalJwtSigners(input map[string][]interface{}) (map[string]string, error) {

	var result = map[string]string{}
	for _, data := range input["externalJwtSigners"] {
		create := FromMap(data, rest_model.ExternalJWTSignerCreate{})

		// see if the signer already exists
		existing := mgmt.ExternalJWTSignerFromFilter(u.client, mgmt.NameFilter(*create.Name))
		if existing != nil {
			if u.verbose {
				log.WithFields(map[string]interface{}{
					"name":                *create.Name,
					"externalJwtSignerId": *existing.ID,
				}).
					Info("Found existing ExtJWTSigner, skipping create")
			}
			_, _ = fmt.Fprintf(u.Err, "\u001B[2KSkipping ExtJWTSigner %s\r", *create.Name)
			continue
		}

		// do the actual create since it doesn't exist
		_, _ = fmt.Fprintf(u.Err, "\u001B[2KCreating ExtJWTSigner %s\r", *create.Name)
		if u.verbose {
			log.WithField("name", *create.Name).Debug("Creating ExtJWTSigner")
		}
		created, createErr := u.client.ExternalJWTSigner.CreateExternalJWTSigner(&external_jwt_signer.CreateExternalJWTSignerParams{ExternalJWTSigner: create}, nil)
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
		if u.verbose {
			log.WithFields(map[string]interface{}{
				"name":                *create.Name,
				"externalJwtSignerId": created.Payload.Data.ID,
			}).
				Info("Created ExtJWTSigner")
		}

		result[*create.Name] = created.Payload.Data.ID
	}

	return result, nil
}
