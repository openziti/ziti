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
	"github.com/google/uuid"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/response"
	"github.com/openziti/ziti/controller/models"
	"github.com/openziti/foundation/v2/stringz"
)

const EntityNameCa = "cas"

var CaLinkFactory = NewCaLinkFactory()

type CaLinkFactoryImpl struct {
	BasicLinkFactory
}

func NewCaLinkFactory() *CaLinkFactoryImpl {
	return &CaLinkFactoryImpl{
		BasicLinkFactory: *NewBasicLinkFactory(EntityNameCa),
	}
}

func (factory *CaLinkFactoryImpl) Links(entity models.Entity) rest_model.Links {
	ca := entity.(*model.Ca)

	links := factory.BasicLinkFactory.Links(entity)
	if ca != nil {
		if !ca.IsVerified {
			links["verify"] = factory.NewNestedLink(entity, "verify")
		}

		if ca.IsAutoCaEnrollmentEnabled {
			links["jwt"] = factory.NewNestedLink(entity, "jwt")
		}
	}

	return links
}

func MapCreateCaToModel(ca *rest_model.CaCreate) *model.Ca {
	ret := &model.Ca{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(ca.Tags),
		},
		Name:                      stringz.OrEmpty(ca.Name),
		Fingerprint:               "",
		CertPem:                   stringz.OrEmpty(ca.CertPem),
		IsVerified:                false,
		VerificationToken:         uuid.New().String(),
		IsAutoCaEnrollmentEnabled: ca.IsAutoCaEnrollmentEnabled != nil && *ca.IsAutoCaEnrollmentEnabled,
		IsOttCaEnrollmentEnabled:  ca.IsOttCaEnrollmentEnabled != nil && *ca.IsOttCaEnrollmentEnabled,
		IsAuthEnabled:             ca.IsAuthEnabled != nil && *ca.IsAuthEnabled,
		IdentityRoles:             ca.IdentityRoles,
		IdentityNameFormat:        ca.IdentityNameFormat,
	}

	if ca.ExternalIDClaim != nil {
		ret.ExternalIdClaim = &model.ExternalIdClaim{}
		ret.ExternalIdClaim.Location = stringz.OrEmpty(ca.ExternalIDClaim.Location)
		ret.ExternalIdClaim.Matcher = stringz.OrEmpty(ca.ExternalIDClaim.Matcher)
		ret.ExternalIdClaim.MatcherCriteria = stringz.OrEmpty(ca.ExternalIDClaim.MatcherCriteria)
		ret.ExternalIdClaim.Parser = stringz.OrEmpty(ca.ExternalIDClaim.Parser)
		ret.ExternalIdClaim.ParserCriteria = stringz.OrEmpty(ca.ExternalIDClaim.ParserCriteria)
		ret.ExternalIdClaim.Index = Int64OrDefault(ca.ExternalIDClaim.Index)
	}

	return ret
}

func MapUpdateCaToModel(id string, ca *rest_model.CaUpdate) *model.Ca {
	ret := &model.Ca{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(ca.Tags),
			Id:   id,
		},
		Name:                      stringz.OrEmpty(ca.Name),
		IsAutoCaEnrollmentEnabled: ca.IsAutoCaEnrollmentEnabled != nil && *ca.IsAutoCaEnrollmentEnabled,
		IsOttCaEnrollmentEnabled:  ca.IsOttCaEnrollmentEnabled != nil && *ca.IsOttCaEnrollmentEnabled,
		IsAuthEnabled:             ca.IsAuthEnabled != nil && *ca.IsAuthEnabled,
		IdentityRoles:             ca.IdentityRoles,
		IdentityNameFormat:        stringz.OrEmpty(ca.IdentityNameFormat),
	}

	if ca.ExternalIDClaim != nil {
		ret.ExternalIdClaim = &model.ExternalIdClaim{}
		ret.ExternalIdClaim.Location = stringz.OrEmpty(ca.ExternalIDClaim.Location)
		ret.ExternalIdClaim.Matcher = stringz.OrEmpty(ca.ExternalIDClaim.Matcher)
		ret.ExternalIdClaim.MatcherCriteria = stringz.OrEmpty(ca.ExternalIDClaim.MatcherCriteria)
		ret.ExternalIdClaim.Parser = stringz.OrEmpty(ca.ExternalIDClaim.Parser)
		ret.ExternalIdClaim.ParserCriteria = stringz.OrEmpty(ca.ExternalIDClaim.ParserCriteria)
		ret.ExternalIdClaim.Index = Int64OrDefault(ca.ExternalIDClaim.Index)
	}

	return ret
}

func MapPatchCaToModel(id string, ca *rest_model.CaPatch) *model.Ca {
	ret := &model.Ca{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(ca.Tags),
			Id:   id,
		},
		Name:                      stringz.OrEmpty(ca.Name),
		IsAutoCaEnrollmentEnabled: BoolOrDefault(ca.IsAutoCaEnrollmentEnabled),
		IsOttCaEnrollmentEnabled:  BoolOrDefault(ca.IsOttCaEnrollmentEnabled),
		IsAuthEnabled:             BoolOrDefault(ca.IsAuthEnabled),
		IdentityRoles:             ca.IdentityRoles,
		IdentityNameFormat:        stringz.OrEmpty(ca.IdentityNameFormat),
	}

	if ca.ExternalIDClaim != nil {
		ret.ExternalIdClaim = &model.ExternalIdClaim{}
		ret.ExternalIdClaim.Location = stringz.OrEmpty(ca.ExternalIDClaim.Location)
		ret.ExternalIdClaim.Matcher = stringz.OrEmpty(ca.ExternalIDClaim.Matcher)
		ret.ExternalIdClaim.MatcherCriteria = stringz.OrEmpty(ca.ExternalIDClaim.MatcherCriteria)
		ret.ExternalIdClaim.Parser = stringz.OrEmpty(ca.ExternalIDClaim.Parser)
		ret.ExternalIdClaim.ParserCriteria = stringz.OrEmpty(ca.ExternalIDClaim.ParserCriteria)
		ret.ExternalIdClaim.Index = Int64OrDefault(ca.ExternalIDClaim.Index)
	}

	return ret
}

func MapCaToRestEntity(_ *env.AppEnv, _ *response.RequestContext, e *model.Ca) (interface{}, error) {
	return MapCaToRestModel(e)
}

func MapCaToRestModel(i *model.Ca) (*rest_model.CaDetail, error) {
	ret := &rest_model.CaDetail{
		BaseEntity:                BaseEntityToRestModel(i, CaLinkFactory),
		CertPem:                   &i.CertPem,
		Fingerprint:               &i.Fingerprint,
		IdentityRoles:             i.IdentityRoles,
		IdentityNameFormat:        &i.IdentityNameFormat,
		IsAuthEnabled:             &i.IsAuthEnabled,
		IsAutoCaEnrollmentEnabled: &i.IsAutoCaEnrollmentEnabled,
		IsOttCaEnrollmentEnabled:  &i.IsOttCaEnrollmentEnabled,
		IsVerified:                &i.IsVerified,
		Name:                      &i.Name,
		VerificationToken:         strfmt.UUID(i.VerificationToken),
	}

	if i.ExternalIdClaim != nil {
		ret.ExternalIDClaim = &rest_model.ExternalIDClaim{
			Index:           &i.ExternalIdClaim.Index,
			Location:        &i.ExternalIdClaim.Location,
			Matcher:         &i.ExternalIdClaim.Matcher,
			MatcherCriteria: &i.ExternalIdClaim.MatcherCriteria,
			Parser:          &i.ExternalIdClaim.Parser,
			ParserCriteria:  &i.ExternalIdClaim.ParserCriteria,
		}
	}

	return ret, nil
}
