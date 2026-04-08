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

package routes

import (
	"github.com/go-openapi/strfmt"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/v2/controller/env"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/response"
)

const EntityNameRevocations = "revocations"

// RevocationLinkFactory is the link factory for revocation entities.
var RevocationLinkFactory = NewBasicLinkFactory(EntityNameRevocations)

// MapRevocationToRestEntity maps a model Revocation to its REST representation for use
// with ListWithHandler and DetailWithHandler.
func MapRevocationToRestEntity(_ *env.AppEnv, _ *response.RequestContext, revocationModel *model.Revocation) (interface{}, error) {
	return MapRevocationToRestModel(revocationModel)
}

// MapRevocationToRestModel converts a model Revocation to a rest_model RevocationDetail.
func MapRevocationToRestModel(revocation *model.Revocation) (*rest_model.RevocationDetail, error) {
	expiresAt := strfmt.DateTime(revocation.ExpiresAt)
	revocationType := rest_model.RevocationTypeEnum(revocation.Type)

	ret := &rest_model.RevocationDetail{
		BaseEntity: BaseEntityToRestModel(revocation, RevocationLinkFactory),
		ExpiresAt:  &expiresAt,
		Type:       &revocationType,
	}

	return ret, nil
}
