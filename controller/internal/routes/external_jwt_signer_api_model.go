/*
	Copyright NetFoundry, Inc.

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
	"fmt"
	"github.com/go-openapi/strfmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/util/stringz"
)

const EntityNameExternalJwtSigner = "external-jwt-signers"

var ExternalJwtSignerLinkFactory = NewBasicLinkFactory(EntityNameExternalJwtSigner)

func MapExternalJwtSignerToRestEntity(_ *env.AppEnv, _ *response.RequestContext, ExternalJwtSignerModel models.Entity) (interface{}, error) {
	externalJwtSigner, ok := ExternalJwtSignerModel.(*model.ExternalJwtSigner)

	if !ok {
		err := fmt.Errorf("entity is not an identity type \"%s\"", ExternalJwtSignerModel.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	restModel := MapExternalJwtSignerToRestModel(externalJwtSigner)

	return restModel, nil
}

func MapClientExtJwtSignersToRestEntity(_ *env.AppEnv, _ *response.RequestContext, signers []*model.ExternalJwtSigner) ([]*rest_model.ClientExternalJWTSignerDetail, error) {
	var ret []*rest_model.ClientExternalJWTSignerDetail

	for _, signer := range signers {
		ret = append(ret, MapClientExternalJwtSignerToRestModel(signer))
	}

	return ret, nil
}

func MapClientExternalJwtSignerToRestModel(externalJwtSigner *model.ExternalJwtSigner) *rest_model.ClientExternalJWTSignerDetail {
	ret := &rest_model.ClientExternalJWTSignerDetail{
		BaseEntity:      BaseEntityToRestModel(externalJwtSigner, ExternalJwtSignerLinkFactory),
		ExternalAuthURL: externalJwtSigner.ExternalAuthUrl,
		Name:            &externalJwtSigner.Name,
	}
	return ret
}

func MapExternalJwtSignerToRestModel(externalJwtSigner *model.ExternalJwtSigner) *rest_model.ExternalJWTSignerDetail {
	notAfter := strfmt.DateTime(externalJwtSigner.NotAfter)
	notBefore := strfmt.DateTime(externalJwtSigner.NotBefore)

	ret := &rest_model.ExternalJWTSignerDetail{
		BaseEntity:      BaseEntityToRestModel(externalJwtSigner, ExternalJwtSignerLinkFactory),
		ClaimsProperty:  externalJwtSigner.ClaimsProperty,
		CommonName:      &externalJwtSigner.CommonName,
		Enabled:         &externalJwtSigner.Enabled,
		ExternalAuthURL: externalJwtSigner.ExternalAuthUrl,
		Fingerprint:     externalJwtSigner.Fingerprint,
		Name:            &externalJwtSigner.Name,
		NotAfter:        &notAfter,
		NotBefore:       &notBefore,
		UseExternalID:   &externalJwtSigner.UseExternalId,
		Kid:             externalJwtSigner.Kid,
		Issuer:          externalJwtSigner.Issuer,
		Audience:        externalJwtSigner.Audience,
		CertPem:         externalJwtSigner.CertPem,
	}

	if externalJwtSigner.JwksEndpoint != nil {
		jwks := strfmt.URI(*externalJwtSigner.JwksEndpoint)
		ret.JwksEndpoint = &jwks
	}

	return ret
}

func MapCreateExternalJwtSignerToModel(signer *rest_model.ExternalJWTSignerCreate) *model.ExternalJwtSigner {
	ret := &model.ExternalJwtSigner{
		BaseEntity:      models.BaseEntity{},
		Name:            *signer.Name,
		Enabled:         *signer.Enabled,
		ExternalAuthUrl: signer.ExternalAuthURL,
		ClaimsProperty:  signer.ClaimsProperty,
		UseExternalId:   BoolOrDefault(signer.UseExternalID),
		Kid:             signer.Kid,
		Issuer:          signer.Issuer,
		Audience:        signer.Audience,
		CertPem:         signer.CertPem,
	}

	if signer.JwksEndpoint != nil {
		jwksEndpoint := signer.JwksEndpoint.String()
		ret.JwksEndpoint = &jwksEndpoint
	}

	return ret
}

func MapUpdateExternalJwtSignerToModel(id string, signer *rest_model.ExternalJWTSignerUpdate) *model.ExternalJwtSigner {
	var tags map[string]interface{}
	if signer.Tags != nil && signer.Tags.SubTags != nil {
		tags = signer.Tags.SubTags
	}

	ret := &model.ExternalJwtSigner{
		BaseEntity: models.BaseEntity{
			Id:       id,
			Tags:     tags,
			IsSystem: false,
		},
		Name:            *signer.Name,
		CertPem:         signer.CertPem,
		Enabled:         *signer.Enabled,
		UseExternalId:   BoolOrDefault(signer.UseExternalID),
		ClaimsProperty:  signer.ClaimsProperty,
		ExternalAuthUrl: signer.ExternalAuthURL,
		Kid:             signer.Kid,
		Issuer:          signer.Issuer,
		Audience:        signer.Audience,
	}

	if signer.JwksEndpoint != nil {
		jwksEndpoint := signer.JwksEndpoint.String()
		ret.JwksEndpoint = &jwksEndpoint
	}

	return ret
}

func MapPatchExternalJwtSignerToModel(id string, signer *rest_model.ExternalJWTSignerPatch) *model.ExternalJwtSigner {
	var tags map[string]interface{}
	if signer.Tags != nil && signer.Tags.SubTags != nil {
		tags = signer.Tags.SubTags
	}

	ret := &model.ExternalJwtSigner{
		BaseEntity: models.BaseEntity{
			Id:       id,
			Tags:     tags,
			IsSystem: false,
		},
		Name:            stringz.OrEmpty(signer.Name),
		CertPem:         signer.CertPem,
		Enabled:         BoolOrDefault(signer.Enabled),
		ExternalAuthUrl: signer.ExternalAuthURL,
		UseExternalId:   BoolOrDefault(signer.UseExternalID),
		ClaimsProperty:  signer.ClaimsProperty,
		Kid:             signer.Kid,
		Issuer:          signer.Issuer,
		Audience:        signer.Audience,
	}

	if signer.JwksEndpoint != nil {
		jwksEndpoint := signer.JwksEndpoint.String()
		ret.JwksEndpoint = &jwksEndpoint
	}

	return ret
}
