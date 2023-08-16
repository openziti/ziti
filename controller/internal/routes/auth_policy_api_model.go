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
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/v2/stringz"
)

const EntityNameAuthPolicy = "auth-policies"

var AuthPolicyLinkFactory = NewBasicLinkFactory(EntityNameAuthPolicy)

func MapAuthPolicyToRestEntity(_ *env.AppEnv, _ *response.RequestContext, authPolicyModel *model.AuthPolicy) (interface{}, error) {
	return MapAuthPolicyToRestModel(authPolicyModel)
}

func MapAuthPolicyToRestModel(model *model.AuthPolicy) (*rest_model.AuthPolicyDetail, error) {
	ret := &rest_model.AuthPolicyDetail{
		BaseEntity: BaseEntityToRestModel(model, AuthPolicyLinkFactory),
		Name:       &model.Name,
		Primary: &rest_model.AuthPolicyPrimary{
			Cert: &rest_model.AuthPolicyPrimaryCert{
				AllowExpiredCerts: &model.Primary.Cert.AllowExpiredCerts,
				Allowed:           &model.Primary.Cert.Allowed,
			},
			ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
				Allowed:        &model.Primary.ExtJwt.Allowed,
				AllowedSigners: model.Primary.ExtJwt.AllowedExtJwtSigners,
			},
			Updb: &rest_model.AuthPolicyPrimaryUpdb{
				Allowed:                &model.Primary.Updb.Allowed,
				MaxAttempts:            &model.Primary.Updb.MaxAttempts,
				MinPasswordLength:      &model.Primary.Updb.MinPasswordLength,
				LockoutDurationMinutes: &model.Primary.Updb.LockoutDurationMinutes,
				RequireMixedCase:       &model.Primary.Updb.RequireMixedCase,
				RequireNumberChar:      &model.Primary.Updb.RequireNumberChar,
				RequireSpecialChar:     &model.Primary.Updb.RequireSpecialChar,
			},
		},
		Secondary: &rest_model.AuthPolicySecondary{
			RequireExtJWTSigner: model.Secondary.RequiredExtJwtSigner,
			RequireTotp:         &model.Secondary.RequireTotp,
		},
	}

	if ret.Primary.ExtJWT.AllowedSigners == nil {
		ret.Primary.ExtJWT.AllowedSigners = []string{}
	}

	return ret, nil
}

func mapCreateAuthPolicyToModel(authPolicy *rest_model.AuthPolicyCreate) *model.AuthPolicy {
	return &model.AuthPolicy{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(authPolicy.Tags),
		},
		Name: *authPolicy.Name,
		Primary: model.AuthPolicyPrimary{
			Cert: model.AuthPolicyCert{
				Allowed:           *authPolicy.Primary.Cert.Allowed,
				AllowExpiredCerts: *authPolicy.Primary.Cert.AllowExpiredCerts,
			},
			Updb: model.AuthPolicyUpdb{
				Allowed:                *authPolicy.Primary.Updb.Allowed,
				MinPasswordLength:      *authPolicy.Primary.Updb.MinPasswordLength,
				RequireSpecialChar:     *authPolicy.Primary.Updb.RequireSpecialChar,
				RequireNumberChar:      *authPolicy.Primary.Updb.RequireNumberChar,
				RequireMixedCase:       *authPolicy.Primary.Updb.RequireMixedCase,
				MaxAttempts:            *authPolicy.Primary.Updb.MaxAttempts,
				LockoutDurationMinutes: *authPolicy.Primary.Updb.LockoutDurationMinutes,
			},
			ExtJwt: model.AuthPolicyExtJwt{
				Allowed:              *authPolicy.Primary.ExtJWT.Allowed,
				AllowedExtJwtSigners: authPolicy.Primary.ExtJWT.AllowedSigners,
			},
		},
		Secondary: model.AuthPolicySecondary{
			RequireTotp:          *authPolicy.Secondary.RequireTotp,
			RequiredExtJwtSigner: authPolicy.Secondary.RequireExtJWTSigner,
		},
	}
}

func MapCreateAuthPolicyToModel(authPolicy *rest_model.AuthPolicyCreate) *model.AuthPolicy {
	return mapCreateAuthPolicyToModel(authPolicy)
}

func MapUpdateAuthPolicyToModel(id string, authPolicy *rest_model.AuthPolicyUpdate) *model.AuthPolicy {
	ret := mapCreateAuthPolicyToModel(&authPolicy.AuthPolicyCreate)
	ret.BaseEntity.Id = id
	return ret
}

func MapPatchAuthPolicyToModel(id string, authPolicy *rest_model.AuthPolicyPatch) *model.AuthPolicy {
	ret := &model.AuthPolicy{
		BaseEntity: models.BaseEntity{
			Id:   id,
			Tags: TagsOrDefault(authPolicy.Tags),
		},
		Name: stringz.OrEmpty(authPolicy.Name),
	}

	if authPolicy.Primary != nil {
		if authPolicy.Primary.Updb != nil {
			ret.Primary.Updb = model.AuthPolicyUpdb{
				Allowed:                BoolOrDefault(authPolicy.Primary.Updb.Allowed),
				MinPasswordLength:      0,
				RequireSpecialChar:     BoolOrDefault(authPolicy.Primary.Updb.RequireSpecialChar),
				RequireNumberChar:      BoolOrDefault(authPolicy.Primary.Updb.RequireMixedCase),
				RequireMixedCase:       BoolOrDefault(authPolicy.Primary.Updb.RequireMixedCase),
				MaxAttempts:            Int64OrDefault(authPolicy.Primary.Updb.MaxAttempts),
				LockoutDurationMinutes: Int64OrDefault(authPolicy.Primary.Updb.LockoutDurationMinutes),
			}
		}

		if authPolicy.Primary.Cert != nil {
			ret.Primary.Cert = model.AuthPolicyCert{
				Allowed:           BoolOrDefault(authPolicy.Primary.Cert.Allowed),
				AllowExpiredCerts: BoolOrDefault(authPolicy.Primary.Cert.AllowExpiredCerts),
			}
		}

		if authPolicy.Primary.ExtJWT != nil {
			ret.Primary.ExtJwt = model.AuthPolicyExtJwt{
				Allowed:              BoolOrDefault(authPolicy.Primary.ExtJWT.Allowed),
				AllowedExtJwtSigners: authPolicy.Primary.ExtJWT.AllowedSigners,
			}
		}
	}

	if authPolicy.Secondary != nil {
		ret.Secondary.RequireTotp = BoolOrDefault(authPolicy.Secondary.RequireTotp)
		ret.Secondary.RequiredExtJwtSigner = authPolicy.Secondary.RequireExtJWTSigner
	}

	return ret
}
