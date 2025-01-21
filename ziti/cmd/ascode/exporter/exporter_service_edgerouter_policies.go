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
	"github.com/openziti/edge-api/rest_management_api_client/service_edge_router_policy"
	"github.com/openziti/edge-api/rest_model"
	"slices"
)

func (exporter Exporter) IsServiceEdgeRouterPolicyExportRequired(args []string) bool {
	return slices.Contains(args, "all") || len(args) == 0 || // explicit all or nothing specified
		slices.Contains(args, "service-edge-router-policy")
}

func (exporter Exporter) GetServiceEdgeRouterPolicies() ([]map[string]interface{}, error) {

	return exporter.getEntities(
		"ServiceEdgeRouterPolicies",

		func() (int64, error) {
			limit := int64(1)
			resp, err := exporter.client.ServiceEdgeRouterPolicy.ListServiceEdgeRouterPolicies(&service_edge_router_policy.ListServiceEdgeRouterPoliciesParams{Limit: &limit}, nil)
			if err != nil {
				return -1, err
			}
			return *resp.GetPayload().Meta.Pagination.TotalCount, nil
		},

		func(offset *int64, limit *int64) ([]interface{}, error) {
			resp, err := exporter.client.ServiceEdgeRouterPolicy.ListServiceEdgeRouterPolicies(&service_edge_router_policy.ListServiceEdgeRouterPoliciesParams{Limit: limit, Offset: offset}, nil)
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

			item := entity.(*rest_model.ServiceEdgeRouterPolicyDetail)

			// convert to a map of values
			m, err := exporter.ToMap(item)
			if err != nil {
				log.WithError(err).Error("error converting ServiceEdgeRouterPolicy to map")
			}

			// translate attributes so they don't reference ids
			serviceRoles := []string{}
			for _, role := range item.ServiceRolesDisplay {
				serviceRoles = append(serviceRoles, role.Name)
			}
			m["serviceRoles"] = serviceRoles
			edgeRouterRoles := []string{}
			for _, role := range item.EdgeRouterRolesDisplay {
				edgeRouterRoles = append(edgeRouterRoles, role.Name)
			}
			m["edgeRouterRoles"] = edgeRouterRoles

			// filter unwanted properties
			exporter.Filter(m, []string{"id", "_links", "createdAt", "updatedAt",
				"edgeRouterRolesDisplay", "serviceRolesDisplay", "isSystem"})

			return m, nil
		})
}
