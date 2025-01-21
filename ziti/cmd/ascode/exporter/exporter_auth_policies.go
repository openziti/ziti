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
	"github.com/openziti/edge-api/rest_management_api_client/auth_policy"
	"github.com/openziti/edge-api/rest_management_api_client/external_jwt_signer"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/internal/ascode"
	"slices"
)

func (exporter Exporter) IsAuthPolicyExportRequired(args []string) bool {
	return slices.Contains(args, "all") || len(args) == 0 || // explicit all or nothing specified
		slices.Contains(args, "auth-policy")
}

func (exporter Exporter) GetAuthPolicies() ([]map[string]interface{}, error) {

	return exporter.getEntities(
		"AuthPolicies",
		func() (int64, error) {
			limit := int64(1)
			resp, err := exporter.client.AuthPolicy.ListAuthPolicies(
				&auth_policy.ListAuthPoliciesParams{Limit: &limit}, nil)
			if err != nil {
				return -1, err
			}
			return *resp.GetPayload().Meta.Pagination.TotalCount, nil

		},
		func(offset *int64, limit *int64) ([]interface{}, error) {
			resp, err := exporter.client.AuthPolicy.ListAuthPolicies(
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
				m, err := exporter.ToMap(item)
				if err != nil {
					log.WithError(err).Error("error converting AuthPolicy input to map")
				}

				// filter unwanted properties
				exporter.Filter(m, []string{"id", "_links", "createdAt", "updatedAt"})

				// deleting Primary so we can reconstruct it
				delete(m, "primary")
				primary, err := exporter.ToMap(item.Primary)
				if err != nil {
					log.WithError(err).Error("error converting AuthPolicy/Primary to map")
				}
				m["primary"] = primary
				// deleting ExtJwt so we can reconstruct it
				delete(primary, "extJwt")
				extJwt, err := exporter.ToMap(item.Primary.ExtJWT)
				if err != nil {
					log.WithError(err).Error("error converting AuthPolicy/Primary/ExtJwtSigner to map")
				}
				primary["extJwt"] = extJwt
				// deleting AllowedSigners because it needs to use a reference to the name instead of the ID
				delete(extJwt, "allowedSigners")
				signers := []string{}
				for _, signer := range item.Primary.ExtJWT.AllowedSigners {
					extJwtSigner, lookupErr := ascode.GetItemFromCache(exporter.externalJwtCache, signer, func(id string) (interface{}, error) {
						return exporter.client.ExternalJWTSigner.DetailExternalJWTSigner(
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
					secondary, err := exporter.ToMap(item.Secondary)
					if err != nil {
						log.WithError(err).Error("error converting AuthPolicy/Secondary to map")
					}
					m["secondary"] = secondary

					// deleting RequiredExtJwtSigner because it needs to use a reference to the name instead of the ID
					delete(secondary, "requireExtJwtSigner")
					extJwtSigner, lookupErr := ascode.GetItemFromCache(exporter.externalJwtCache, *item.Secondary.RequireExtJWTSigner, func(id string) (interface{}, error) {
						return exporter.client.ExternalJWTSigner.DetailExternalJWTSigner(&external_jwt_signer.DetailExternalJWTSignerParams{ID: id}, nil)
					})
					if lookupErr != nil {
						return nil, lookupErr
					}
					secondary["requireExtJwtSigner"] = "@" + *extJwtSigner.(*external_jwt_signer.DetailExternalJWTSignerOK).Payload.Data.Name
				}

				return m, nil
			}
			return nil, nil
		})

}
