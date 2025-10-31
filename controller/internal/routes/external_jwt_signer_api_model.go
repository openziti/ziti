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
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/models"
	"github.com/openziti/ziti/controller/response"
)

const EntityNameExternalJwtSigner = "external-jwt-signers"

var ExternalJwtSignerLinkFactory = NewBasicLinkFactory(EntityNameExternalJwtSigner)

func MapExternalJwtSignerToRestEntityForManagement(_ *env.AppEnv, _ *response.RequestContext, externalJwtSigner *model.ExternalJwtSigner) (interface{}, error) {
	return MapExternalJwtSignerToRestModelForManagement(externalJwtSigner), nil
}

func MapExtJwtSignersToRestEntityForClient(_ *env.AppEnv, _ *response.RequestContext, signers []*model.ExternalJwtSigner) ([]*rest_model.ClientExternalJWTSignerDetail, error) {
	var ret []*rest_model.ClientExternalJWTSignerDetail

	for _, signer := range signers {
		ret = append(ret, MapExternalJwtSignerToRestModelForClient(signer))
	}

	return ret, nil
}

func MapExternalJwtSignerToRestModelForClient(externalJwtSigner *model.ExternalJwtSigner) *rest_model.ClientExternalJWTSignerDetail {
	targetToken := rest_model.TargetToken(externalJwtSigner.TargetToken)

	ret := &rest_model.ClientExternalJWTSignerDetail{
		BaseEntity:           BaseEntityToRestModel(externalJwtSigner, ExternalJwtSignerLinkFactory),
		Audience:             externalJwtSigner.Audience,
		ClientID:             externalJwtSigner.ClientId,
		EnrollToCertEnabled:  externalJwtSigner.EnrollToCertEnabled,
		EnrollToTokenEnabled: externalJwtSigner.EnrollToTokenEnabled,
		ExternalAuthURL:      externalJwtSigner.ExternalAuthUrl,
		Name:                 &externalJwtSigner.Name,
		Scopes:               externalJwtSigner.Scopes,
		TargetToken:          &targetToken,
	}
	return ret
}

func MapExternalJwtSignerToRestModelForManagement(externalJwtSigner *model.ExternalJwtSigner) *rest_model.ExternalJWTSignerDetail {
	notAfter := strfmt.DateTime(externalJwtSigner.NotAfter)
	notBefore := strfmt.DateTime(externalJwtSigner.NotBefore)
	targetToken := rest_model.TargetToken(externalJwtSigner.TargetToken)
	ret := &rest_model.ExternalJWTSignerDetail{
		BaseEntity:                    BaseEntityToRestModel(externalJwtSigner, ExternalJwtSignerLinkFactory),
		Audience:                      externalJwtSigner.Audience,
		CertPem:                       externalJwtSigner.CertPem,
		ClaimsProperty:                externalJwtSigner.ClaimsProperty,
		ClientID:                      externalJwtSigner.ClientId,
		CommonName:                    &externalJwtSigner.CommonName,
		Enabled:                       &externalJwtSigner.Enabled,
		EnrollAttributeClaimsSelector: externalJwtSigner.EnrollAttributeClaimsSelector,
		EnrollNameClaimsSelector:      externalJwtSigner.EnrollNameClaimselector,
		EnrollAuthPolicyID:            externalJwtSigner.EnrollAuthPolicyId,
		EnrollToCertEnabled:           externalJwtSigner.EnrollToCertEnabled,
		EnrollToTokenEnabled:          externalJwtSigner.EnrollToTokenEnabled,
		ExternalAuthURL:               externalJwtSigner.ExternalAuthUrl,
		Fingerprint:                   externalJwtSigner.Fingerprint,
		Issuer:                        externalJwtSigner.Issuer,
		JwksEndpoint:                  nil,
		Kid:                           externalJwtSigner.Kid,
		Name:                          &externalJwtSigner.Name,
		NotAfter:                      &notAfter,
		NotBefore:                     &notBefore,
		Scopes:                        externalJwtSigner.Scopes,
		TargetToken:                   &targetToken,
		UseExternalID:                 &externalJwtSigner.UseExternalId,
	}

	if externalJwtSigner.JwksEndpoint != nil {
		jwks := strfmt.URI(*externalJwtSigner.JwksEndpoint)
		ret.JwksEndpoint = &jwks
	}

	return ret
}

func MapCreateExternalJwtSignerToModelForManagement(signer *rest_model.ExternalJWTSignerCreate) *model.ExternalJwtSigner {
	targetToken := string(rest_model.TargetTokenACCESS)

	if signer.TargetToken != nil {
		targetToken = string(*signer.TargetToken)
		if targetToken == "" {
			targetToken = string(rest_model.TargetTokenACCESS)
		}
	}

	ret := &model.ExternalJwtSigner{
		BaseEntity:                    models.BaseEntity{},
		Name:                          *signer.Name,
		Enabled:                       *signer.Enabled,
		ExternalAuthUrl:               signer.ExternalAuthURL,
		ClaimsProperty:                signer.ClaimsProperty,
		UseExternalId:                 BoolOrDefault(signer.UseExternalID),
		Kid:                           signer.Kid,
		Issuer:                        signer.Issuer,
		Audience:                      signer.Audience,
		CertPem:                       signer.CertPem,
		ClientId:                      signer.ClientID,
		Scopes:                        signer.Scopes,
		TargetToken:                   targetToken,
		EnrollToCertEnabled:           signer.EnrollToCertEnabled,
		EnrollToTokenEnabled:          signer.EnrollToTokenEnabled,
		EnrollAttributeClaimsSelector: signer.EnrollAttributeClaimsSelector,
		EnrollNameClaimselector:       signer.EnrollNameClaimsSelector,
		EnrollAuthPolicyId:            signer.EnrollAuthPolicyID,
	}

	if signer.JwksEndpoint != nil {
		jwksEndpoint := signer.JwksEndpoint.String()
		ret.JwksEndpoint = &jwksEndpoint
	}

	return ret
}

func MapUpdateExternalJwtSignerToModelForManagement(id string, signer *rest_model.ExternalJWTSignerUpdate) *model.ExternalJwtSigner {
	var tags map[string]interface{}
	if signer.Tags != nil && signer.Tags.SubTags != nil {
		tags = signer.Tags.SubTags
	}

	targetToken := string(rest_model.TargetTokenACCESS)

	if signer.TargetToken != nil {
		targetToken = string(*signer.TargetToken)
		if targetToken == "" {
			targetToken = string(rest_model.TargetTokenACCESS)
		}
	}

	ret := &model.ExternalJwtSigner{
		BaseEntity: models.BaseEntity{
			Id:       id,
			Tags:     tags,
			IsSystem: false,
		},
		Name:                          *signer.Name,
		CertPem:                       signer.CertPem,
		Enabled:                       *signer.Enabled,
		UseExternalId:                 BoolOrDefault(signer.UseExternalID),
		ClaimsProperty:                signer.ClaimsProperty,
		ExternalAuthUrl:               signer.ExternalAuthURL,
		Kid:                           signer.Kid,
		Issuer:                        signer.Issuer,
		Audience:                      signer.Audience,
		ClientId:                      signer.ClientID,
		Scopes:                        signer.Scopes,
		TargetToken:                   targetToken,
		EnrollToCertEnabled:           BoolOrDefault(signer.EnrollToCertEnabled),
		EnrollToTokenEnabled:          BoolOrDefault(signer.EnrollToTokenEnabled),
		EnrollAttributeClaimsSelector: stringz.OrEmpty(signer.EnrollAttributeClaimsSelector),
		EnrollNameClaimselector:       stringz.OrEmpty(signer.EnrollNameClaimsSelector),
		EnrollAuthPolicyId:            stringz.OrEmpty(signer.EnrollAuthPolicyID),
	}

	if signer.JwksEndpoint != nil {
		jwksEndpoint := signer.JwksEndpoint.String()
		ret.JwksEndpoint = &jwksEndpoint
	}

	return ret
}

func MapPatchExternalJwtSignerToModelForManagement(id string, signer *rest_model.ExternalJWTSignerPatch) *model.ExternalJwtSigner {
	var tags map[string]interface{}
	if signer.Tags != nil && signer.Tags.SubTags != nil {
		tags = signer.Tags.SubTags
	}

	targetToken := string(rest_model.TargetTokenACCESS)

	if signer.TargetToken != nil {
		targetToken = string(*signer.TargetToken)
		if targetToken == "" {
			targetToken = string(rest_model.TargetTokenACCESS)
		}
	}

	ret := &model.ExternalJwtSigner{
		BaseEntity: models.BaseEntity{
			Id:       id,
			Tags:     tags,
			IsSystem: false,
		},
		Name:                          stringz.OrEmpty(signer.Name),
		CertPem:                       signer.CertPem,
		Enabled:                       BoolOrDefault(signer.Enabled),
		ExternalAuthUrl:               signer.ExternalAuthURL,
		UseExternalId:                 BoolOrDefault(signer.UseExternalID),
		ClaimsProperty:                signer.ClaimsProperty,
		Kid:                           signer.Kid,
		Issuer:                        signer.Issuer,
		Audience:                      signer.Audience,
		ClientId:                      signer.ClientID,
		Scopes:                        signer.Scopes,
		TargetToken:                   targetToken,
		EnrollToCertEnabled:           BoolOrDefault(signer.EnrollToCertEnabled),
		EnrollToTokenEnabled:          BoolOrDefault(signer.EnrollToTokenEnabled),
		EnrollAttributeClaimsSelector: stringz.OrEmpty(signer.EnrollAttributeClaimsSelector),
		EnrollNameClaimselector:       stringz.OrEmpty(signer.EnrollNameClaimsSelector),
		EnrollAuthPolicyId:            stringz.OrEmpty(signer.EnrollAuthPolicyID),
	}

	if signer.JwksEndpoint != nil {
		jwksEndpoint := signer.JwksEndpoint.String()
		ret.JwksEndpoint = &jwksEndpoint
	}

	return ret
}
