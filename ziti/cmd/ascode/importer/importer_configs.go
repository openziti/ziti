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
	"github.com/openziti/edge-api/rest_management_api_client"
	"github.com/openziti/edge-api/rest_management_api_client/config"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	"github.com/openziti/ziti/internal"
	"github.com/openziti/ziti/internal/ascode"
	"github.com/openziti/ziti/internal/rest/mgmt"
	"slices"
)

func (importer *Importer) IsConfigImportRequired(args []string) bool {
	return slices.Contains(args, "all") || len(args) == 0 || // explicit all or nothing specified
		slices.Contains(args, "config") ||
		slices.Contains(args, "service")
}

func (importer *Importer) ProcessConfigs(client *rest_management_api_client.ZitiEdgeManagement, input map[string][]interface{}) (map[string]string, error) {

	var result = map[string]string{}
	for _, data := range input["configs"] {
		create := FromMap(data, rest_model.ConfigCreate{})

		// see if the config already exists
		existing := mgmt.ConfigFromFilter(client, mgmt.NameFilter(*create.Name))
		if existing != nil {
			log.
				WithFields(map[string]interface{}{
					"name":     *create.Name,
					"configId": *existing.ID,
				}).
				Info("Found existing Config, skipping create")
			_, _ = internal.FPrintfReusingLine(importer.Err, "Skipping Config %s\r", *create.Name)
			continue
		}

		// convert to a json doc so we can query inside the data
		jsonData, _ := json.Marshal(data)
		doc, jsonParseError := gabs.ParseJSON(jsonData)
		if jsonParseError != nil {
			log.WithError(jsonParseError).Error("Unable to parse json")
			return nil, jsonParseError
		}

		// look up the config type id from the name and add to the create
		value := doc.Path("configType").Data().(string)[1:]
		configType, _ := ascode.GetItemFromCache(importer.configCache, value, func(name string) (interface{}, error) {
			return mgmt.ConfigTypeFromFilter(client, mgmt.NameFilter(name)), nil
		})
		if importer.configCache == nil {
			return nil, errors.New("error reading ConfigType: " + value)
		}
		create.ConfigTypeID = configType.(*rest_model.ConfigTypeDetail).ID

		// do the actual create since it doesn't exist
		_, _ = internal.FPrintfReusingLine(importer.Err, "Creating Config %s\r", *create.Name)
		log.WithField("name", *create.Name).Debug("Creating Config")
		created, createErr := client.Config.CreateConfig(&config.CreateConfigParams{Config: create}, nil)
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
		log.WithFields(map[string]interface{}{
			"name":     *create.Name,
			"configId": created.Payload.Data.ID,
		}).
			Info("Created Config")
		result[*create.Name] = created.Payload.Data.ID
	}

	return result, nil
}
