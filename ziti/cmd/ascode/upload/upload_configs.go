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
	"github.com/openziti/edge-api/rest_management_api_client/config"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	common "github.com/openziti/ziti/internal/ascode"
	"github.com/openziti/ziti/internal/rest/mgmt"
	"strings"
)

func (u *Upload) ProcessConfigs(input map[string][]interface{}) (map[string]string, error) {

	var result = map[string]string{}
	for _, data := range input["configs"] {
		create := FromMap(data, rest_model.ConfigCreate{})

		// see if the config already exists
		existing := mgmt.ConfigFromFilter(u.client, mgmt.NameFilter(*create.Name))
		if existing != nil {
			if u.verbose {
				log.
					WithFields(map[string]interface{}{
						"name":     *create.Name,
						"configId": *existing.ID,
					}).
					Info("Found existing Config, skipping create")
			}
			continue
		}

		// convert to a jsonquery doc so we can query inside the json
		jsonData, _ := json.Marshal(data)
		doc, jsonQueryErr := jsonquery.Parse(strings.NewReader(string(jsonData)))
		if jsonQueryErr != nil {
			log.WithError(jsonQueryErr).Error("Unable to parse json")
			return nil, jsonQueryErr
		}

		// look up the config type id from the name and add to the create
		value := jsonquery.FindOne(doc, "/configType").Value().(string)[1:]
		configType, _ := common.GetItemFromCache(u.configCache, value, func(name string) (interface{}, error) {
			return mgmt.ConfigTypeFromFilter(u.client, mgmt.NameFilter(name)), nil
		})
		if u.configCache == nil {
			return nil, errors.New("error reading ConfigType: " + value)
		}
		create.ConfigTypeID = configType.(*rest_model.ConfigTypeDetail).ID

		// do the actual create since it doesn't exist
		if u.verbose {
			log.WithField("name", *create.Name).Debug("Creating Config")
		}
		created, createErr := u.client.Config.CreateConfig(&config.CreateConfigParams{Config: create}, nil)
		if createErr != nil {
			if payloadErr, ok := createErr.(rest_util.ApiErrorPayload); ok {
				log.WithFields(map[string]interface{}{
					"field":  payloadErr.GetPayload().Error.Cause.APIFieldError.Field,
					"reason": payloadErr.GetPayload().Error.Cause.APIFieldError.Reason}).
					Error("Unable to create Config")
				return nil, createErr
			} else {
				log.WithError(createErr).Error("Unable to list Configs")
				return nil, createErr
			}
		}
		if u.verbose {
			log.WithFields(map[string]interface{}{
				"name":     *create.Name,
				"configId": created.Payload.Data.ID,
			}).
				Info("Created Config")
		}
		result[*create.Name] = created.Payload.Data.ID
	}

	return result, nil
}
