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
	"github.com/openziti/edge-api/rest_management_api_client/config"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	"github.com/openziti/ziti/internal/rest/mgmt"
)

func (u *Upload) ProcessConfigTypes(input map[string][]interface{}) (map[string]string, error) {

	var result = map[string]string{}
	for _, data := range input["configTypes"] {
		create := FromMap(data, rest_model.ConfigTypeCreate{})

		// see if the config type already exists
		existing := mgmt.ConfigTypeFromFilter(u.client, mgmt.NameFilter(*create.Name))
		if existing != nil {
			if u.verbose {
				log.WithFields(map[string]interface{}{
					"name":         *create.Name,
					"configTypeId": *existing.ID,
				}).
					Info("Found existing ConfigType, skipping create")
			}
			continue
		}

		// do the actual create since it doesn't exist
		if u.verbose {
			log.WithField("name", *create.Name).
				Debug("Creating ConfigType")
		}
		created, createErr := u.client.Config.CreateConfigType(&config.CreateConfigTypeParams{ConfigType: create}, nil)
		if createErr != nil {
			if payloadErr, ok := createErr.(rest_util.ApiErrorPayload); ok {
				log.WithFields(map[string]interface{}{
					"field":  payloadErr.GetPayload().Error.Cause.APIFieldError.Field,
					"reason": payloadErr.GetPayload().Error.Cause.APIFieldError.Reason,
				}).
					Error("Unable to create ConfigType")
			} else {
				log.WithError(createErr).
					Error("Unable to create ConfigType")
			}
			return nil, createErr
		}

		if u.verbose {
			log.WithFields(map[string]interface{}{
				"name":         *create.Name,
				"configTypeId": created.Payload.Data.ID,
			}).
				Info("Created Config Type")
		}

		result[*create.Name] = created.Payload.Data.ID
	}

	return result, nil
}
