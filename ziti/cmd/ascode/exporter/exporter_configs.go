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

package exporter

import (
	"errors"
	"github.com/openziti/edge-api/rest_management_api_client/config"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/internal/ascode"
)

func (d Exporter) GetConfigs() ([]map[string]interface{}, error) {

	return d.getEntities(
		"Configs",

		func() (int64, error) {
			limit := int64(1)
			resp, err := d.client.Config.ListConfigs(&config.ListConfigsParams{Limit: &limit}, nil)
			if err != nil {
				return -1, err
			}
			return *resp.GetPayload().Meta.Pagination.TotalCount, nil
		},

		func(offset *int64, limit *int64) ([]interface{}, error) {
			resp, _ := d.client.Config.ListConfigs(&config.ListConfigsParams{Limit: limit, Offset: offset}, nil)
			entities := make([]interface{}, len(resp.GetPayload().Data))
			for i, c := range resp.GetPayload().Data {
				entities[i] = interface{}(c)
			}
			return entities, nil
		},

		func(entity interface{}) (map[string]interface{}, error) {

			item := entity.(*rest_model.ConfigDetail)

			// convert to a map of values
			m := d.ToMap(item)

			// filter unwanted properties
			d.Filter(m, []string{"id", "_links", "createdAt", "updatedAt"})

			// translate ids to names
			delete(m, "configType")
			delete(m, "configTypeId")
			configType, lookupErr := ascode.GetItemFromCache(d.configTypeCache, *item.ConfigTypeID, func(id string) (interface{}, error) {
				return d.client.Config.DetailConfigType(&config.DetailConfigTypeParams{ID: id}, nil)
			})
			if lookupErr != nil {
				return nil, errors.Join(errors.New("error reading Auth Policy: "+*item.ConfigTypeID), lookupErr)
			}
			m["configType"] = "@" + *configType.(*config.DetailConfigTypeOK).Payload.Data.Name

			return m, nil
		})
}
