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
	"fmt"
	"github.com/antchfx/jsonquery"
	"github.com/openziti/edge-api/rest_management_api_client/service"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	common "github.com/openziti/ziti/internal/ascode"
	"github.com/openziti/ziti/internal/rest/mgmt"
	"strings"
)

func (u *Upload) ProcessServices(input map[string][]interface{}) (map[string]string, error) {

	var result = map[string]string{}

	for _, data := range input["services"] {
		create := FromMap(data, rest_model.ServiceCreate{})

		// see if the service already exists
		existing := mgmt.ServiceFromFilter(u.client, mgmt.NameFilter(*create.Name))
		if existing != nil {
			log.WithFields(map[string]interface{}{
				"name":      *create.Name,
				"serviceId": *existing.ID,
			}).
				Info("Found existing Service, skipping create")
			_, _ = fmt.Fprintf(u.Err, "\u001B[2KSkipping Service %s\r", *create.Name)
			continue
		}

		// convert to a jsonquery doc so we can query inside the json
		jsonData, _ := json.Marshal(data)
		doc, jsonQueryErr := jsonquery.Parse(strings.NewReader(string(jsonData)))
		if jsonQueryErr != nil {
			log.WithError(jsonQueryErr).Error("Unable to ")
			return nil, jsonQueryErr
		}
		configsNode := jsonquery.FindOne(doc, "/configs")

		// look up each config by name and add to the create
		configIds := []string{}
		for _, configName := range configsNode.ChildNodes() {
			value := configName.Value().(string)[1:]
			config, _ := common.GetItemFromCache(u.configCache, value, func(name string) (interface{}, error) {
				return mgmt.ConfigFromFilter(u.client, mgmt.NameFilter(name)), nil
			})
			if config == nil {
				return nil, errors.New("error reading Config: " + value)
			}
			configIds = append(configIds, *config.(*rest_model.ConfigDetail).ID)
		}
		create.Configs = configIds

		// do the actual create since it doesn't exist
		_, _ = fmt.Fprintf(u.Err, "\u001B[2KCreating Service %s\r", *create.Name)
		if u.verbose {
			log.WithField("name", *create.Name).Debug("Creating Service")
		}
		created, createErr := u.client.Service.CreateService(&service.CreateServiceParams{Service: create}, nil)
		if createErr != nil {
			if payloadErr, ok := createErr.(rest_util.ApiErrorPayload); ok {
				log.WithFields(map[string]interface{}{
					"field":  payloadErr.GetPayload().Error.Cause.APIFieldError.Field,
					"reason": payloadErr.GetPayload().Error.Cause.APIFieldError.Reason,
				}).
					Error("Unable to create Service")
			} else {
				log.WithError(createErr).Error("Unable to create Service")
				return nil, createErr
			}
		}
		if u.verbose {
			log.WithFields(map[string]interface{}{
				"name":      *create.Name,
				"serviceId": created.Payload.Data.ID,
			}).
				Info("Created Service")
		}

		result[*create.Name] = created.Payload.Data.ID
	}

	return result, nil
}

func (u *Upload) lookupServices(roles []string) ([]string, error) {
	serviceRoles := []string{}
	for _, role := range roles {
		if role[0:1] == "@" {
			value := role[1:]
			service, _ := common.GetItemFromCache(u.serviceCache, value, func(name string) (interface{}, error) {
				return mgmt.ServiceFromFilter(u.client, mgmt.NameFilter(name)), nil
			})
			if service == nil {
				return nil, errors.New("error reading Service: " + value)
			}
			serviceId := service.(*rest_model.ServiceDetail).ID
			serviceRoles = append(serviceRoles, "@"+*serviceId)
		} else {
			serviceRoles = append(serviceRoles, role)
		}
	}
	return serviceRoles, nil
}
