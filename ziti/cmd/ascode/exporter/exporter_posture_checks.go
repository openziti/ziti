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
	"github.com/openziti/edge-api/rest_management_api_client/posture_checks"
	"golang.org/x/exp/slices"
)

func (exporter Exporter) IsPostureCheckExportRequired(args []string) bool {
	return slices.Contains(args, "all") || len(args) == 0 || // explicit all or nothing specified
		slices.Contains(args, "posture-check")
}

func (exporter Exporter) GetPostureChecks() ([]map[string]interface{}, error) {

	return exporter.getEntities(
		"PostureChecks",

		func() (int64, error) {
			limit := int64(1)
			resp, err := exporter.client.PostureChecks.ListPostureChecks(&posture_checks.ListPostureChecksParams{Limit: &limit}, nil)
			if err != nil {
				return -1, err
			}
			return *resp.GetPayload().Meta.Pagination.TotalCount, nil
		},

		func(offset *int64, limit *int64) ([]interface{}, error) {
			resp, _ := exporter.client.PostureChecks.ListPostureChecks(&posture_checks.ListPostureChecksParams{Limit: limit, Offset: offset}, nil)
			entities := make([]interface{}, len(resp.GetPayload().Data()))
			for i, c := range resp.GetPayload().Data() {
				entities[i] = interface{}(c)
			}
			return entities, nil
		},

		func(entity interface{}) (map[string]interface{}, error) {

			// convert to a map of values
			m := exporter.ToMap(entity)

			// filter unwanted properties
			exporter.Filter(m, []string{"id", "_links", "createdAt", "updatedAt",
				"version"})

			return m, nil
		})
}
