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
	"github.com/openziti/edge-api/rest_management_api_client/edge_router_policy"
	"github.com/openziti/edge-api/rest_model"
)

func (d Exporter) GetRouterPolicies() ([]map[string]interface{}, error) {

	return d.getEntities(
		"EdgeRouterPolicies",

		func() (int64, error) {
			limit := int64(1)
			resp, err := d.client.EdgeRouterPolicy.ListEdgeRouterPolicies(&edge_router_policy.ListEdgeRouterPoliciesParams{Limit: &limit}, nil)
			if err != nil {
				return -1, err
			}
			return *resp.GetPayload().Meta.Pagination.TotalCount, nil
		},

		func(offset *int64, limit *int64) ([]interface{}, error) {
			resp, err := d.client.EdgeRouterPolicy.ListEdgeRouterPolicies(&edge_router_policy.ListEdgeRouterPoliciesParams{Limit: limit, Offset: offset}, nil)
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

			item := entity.(*rest_model.EdgeRouterPolicyDetail)

			// convert to a map of values
			m := d.ToMap(item)

			// translate attributes so they don't reference ids
			identityRoles := []string{}
			for _, role := range item.IdentityRolesDisplay {
				identityRoles = append(identityRoles, role.Name)
			}
			m["identityRoles"] = identityRoles
			edgeRouterRoles := []string{}
			for _, role := range item.EdgeRouterRolesDisplay {
				edgeRouterRoles = append(edgeRouterRoles, role.Name)
			}
			m["edgeRouterRoles"] = edgeRouterRoles

			// filter unwanted properties
			d.Filter(m, []string{"id", "_links", "createdAt", "updatedAt",
				"edgeRouterRolesDisplay", "identityRolesDisplay", "isSystem"})

			return m, nil
		})
}
