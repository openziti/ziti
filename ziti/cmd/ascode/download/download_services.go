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

package download

import (
	"errors"
	"github.com/openziti/edge-api/rest_management_api_client/config"
	"github.com/openziti/edge-api/rest_management_api_client/service"
	"github.com/openziti/edge-api/rest_model"
	common "github.com/openziti/ziti/internal/ascode"
)

func (d Download) GetServices() ([]map[string]interface{}, error) {

	return d.getEntities(
		"Services",

		func() (int64, error) {
			limit := int64(1)
			resp, err := d.client.Service.ListServices(&service.ListServicesParams{Limit: &limit}, nil)
			if err != nil {
				return -1, err
			}
			return *resp.GetPayload().Meta.Pagination.TotalCount, nil
		},

		func(offset *int64, limit *int64) ([]interface{}, error) {
			resp, err := d.client.Service.ListServices(&service.ListServicesParams{Limit: limit, Offset: offset}, nil)
			if err != nil {
				return nil, err
			}
			entities := make([]interface{}, len(resp.GetPayload().Data))
			for i, c := range resp.GetPayload().Data {
				entities[i] = interface{}(c)
			}
			return entities, nil
		},

		func(entity interface{}) (map[string]interface{}, error) {

			item := entity.(*rest_model.ServiceDetail)

			// convert to a map of values
			m := d.ToMap(item)

			d.defaultRoleAttributes(m)

			// filter unwanted properties
			d.Filter(m, []string{"id", "_links", "createdAt", "updatedAt",
				"configs", "config", "data", "postureQueries", "permissions", "maxIdleTimeMillis"})

			// translate ids to names
			var configNames []string
			for _, c := range item.Configs {
				configDetail, lookupErr := common.GetItemFromCache(d.configCache, c, func(id string) (interface{}, error) {
					return d.client.Config.DetailConfig(&config.DetailConfigParams{ID: id}, nil)
				})
				if lookupErr != nil {
					return nil, errors.Join(errors.New("error reading Config: "+c), lookupErr)
				}
				configNames = append(configNames, "@"+*configDetail.(*config.DetailConfigOK).Payload.Data.Name)
			}
			delete(m, "configs")
			m["configs"] = configNames

			return m, nil
		})
}
