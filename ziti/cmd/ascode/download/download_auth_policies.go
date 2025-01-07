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
	"github.com/openziti/edge-api/rest_management_api_client/auth_policy"
	"github.com/openziti/edge-api/rest_management_api_client/external_jwt_signer"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/internal/ascode"
)

func (d Download) GetAuthPolicies() ([]map[string]interface{}, error) {

	return d.getEntities(
		"AuthPolicies",
		func() (int64, error) {
			limit := int64(1)
			resp, err := d.client.AuthPolicy.ListAuthPolicies(
				&auth_policy.ListAuthPoliciesParams{Limit: &limit}, nil)
			if err != nil {
				return -1, err
			}
			return *resp.GetPayload().Meta.Pagination.TotalCount, nil

		},
		func(offset *int64, limit *int64) ([]interface{}, error) {
			resp, err := d.client.AuthPolicy.ListAuthPolicies(
				&auth_policy.ListAuthPoliciesParams{Limit: limit, Offset: offset}, nil)
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

			item := entity.(*rest_model.AuthPolicyDetail)

			if *item.Name != "Default" {
				// convert to a map of values
				m := d.ToMap(item)

				// filter unwanted properties
				d.Filter(m, []string{"id", "_links", "createdAt", "updatedAt"})

				// deleting Primary so we can reconstruct it
				delete(m, "primary")
				primary := d.ToMap(item.Primary)
				m["primary"] = primary
				// deleting ExtJwt so we can reconstruct it
				delete(primary, "extJwt")
				extJwt := d.ToMap(item.Primary.ExtJWT)
				primary["extJwt"] = extJwt
				// deleting AllowedSigners because it needs to use a reference to the name instead of the ID
				delete(extJwt, "allowedSigners")
				signers := []string{}
				for _, signer := range item.Primary.ExtJWT.AllowedSigners {
					extJwtSigner, lookupErr := ascode.GetItemFromCache(d.externalJwtCache, signer, func(id string) (interface{}, error) {
						return d.client.ExternalJWTSigner.DetailExternalJWTSigner(
							&external_jwt_signer.DetailExternalJWTSignerParams{ID: id}, nil)
					})
					if lookupErr != nil {
						return nil, lookupErr
					}
					signers = append(signers, "@"+*extJwtSigner.(*external_jwt_signer.DetailExternalJWTSignerOK).Payload.Data.Name)
				}
				extJwt["allowedSigners"] = signers

				// if a secondary jwt signer is set, update it with a name reference instead of the id
				if item.Secondary.RequireExtJWTSigner != nil {
					// deleting Secondary so we can reconstruct it
					delete(m, "secondary")
					secondary := d.ToMap(item.Secondary)
					m["secondary"] = secondary

					// deleting RequiredExtJwtSigner because it needs to use a reference to the name instead of the ID
					delete(secondary, "requiredExtJwtSigner")
					requiredExtJwtSigner := d.ToMap(item.Secondary.RequireExtJWTSigner)
					extJwtSigner, lookupErr := ascode.GetItemFromCache(d.externalJwtCache, *item.Secondary.RequireExtJWTSigner, func(id string) (interface{}, error) {
						return d.client.ExternalJWTSigner.DetailExternalJWTSigner(&external_jwt_signer.DetailExternalJWTSignerParams{ID: id}, nil)
					})
					if lookupErr != nil {
						return nil, lookupErr
					}
					requiredExtJwtSigner["requiredExtJwtSigner"] = "@" + *extJwtSigner.(*external_jwt_signer.DetailExternalJWTSignerOK).Payload.Data.Name
				}

				return m, nil
			}
			return nil, nil
		})

}
