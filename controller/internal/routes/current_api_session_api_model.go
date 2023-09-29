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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/response"
	"github.com/openziti/ziti/controller/models"
	"time"
)

const EntityNameCurrentSession = "current-api-session"
const EntityNameCurrentSessionCertificates = "certificates"

var CurrentApiSessionCertificateLinkFactory *BasicLinkFactory = NewBasicLinkFactory(EntityNameCurrentSession + "/" + EntityNameCurrentSessionCertificates)

var CurrentApiSessionLinkFactory LinksFactory = NewCurrentApiSessionLinkFactory()

type CurrentApiSessionLinkFactoryImpl struct {
	BasicLinkFactory
}

func NewCurrentApiSessionLinkFactory() *CurrentApiSessionLinkFactoryImpl {
	return &CurrentApiSessionLinkFactoryImpl{
		BasicLinkFactory: *NewBasicLinkFactory(EntityNameCurrentSession),
	}
}

func (factory *CurrentApiSessionLinkFactoryImpl) SelfLink(entity models.Entity) rest_model.Link {
	return NewLink("./" + EntityNameCurrentSession)
}

func (factory *CurrentApiSessionLinkFactoryImpl) Links(entity models.Entity) rest_model.Links {
	return rest_model.Links{
		EntityNameSelf:            factory.SelfLink(entity),
		EntityNameCurrentIdentity: CurrentIdentityLinkFactory.SelfLink(entity),
	}
}

func MapToCurrentApiSessionRestModel(ae *env.AppEnv, rc *response.RequestContext, sessionTimeout time.Duration) *rest_model.CurrentAPISessionDetail {

	detail, err := MapApiSessionToRestModel(ae, rc.ApiSession)

	MapApiSessionAuthQueriesToRestEntity(ae, rc, detail)

	if err != nil {
		pfxlog.Logger().Errorf("error could not convert apiSession to rest model: %v", err)
	}

	if detail == nil {
		detail = &rest_model.APISessionDetail{}
	}
	expiresAt := strfmt.DateTime(time.Time(detail.LastActivityAt).Add(sessionTimeout))
	expirationSeconds := int64(rc.ApiSession.ExpirationDuration.Seconds())

	ret := &rest_model.CurrentAPISessionDetail{
		APISessionDetail:  *detail,
		ExpiresAt:         &expiresAt,
		ExpirationSeconds: &expirationSeconds,
	}

	return ret
}

func MapApiSessionAuthQueriesToRestEntity(ae *env.AppEnv, rc *response.RequestContext, detail *rest_model.APISessionDetail) {
	for _, authQuery := range rc.AuthQueries {
		detail.AuthQueries = append(detail.AuthQueries, &rest_model.AuthQueryDetail{
			Format:     authQuery.Format,
			HTTPMethod: authQuery.HTTPMethod,
			HTTPURL:    authQuery.HTTPURL,
			MaxLength:  authQuery.MaxLength,
			MinLength:  authQuery.MinLength,
			Provider:   authQuery.Provider,
			TypeID:     authQuery.TypeID,
		})
	}
}

func MapApiSessionCertificateToRestEntity(appEnv *env.AppEnv, context *response.RequestContext, cert *model.ApiSessionCertificate) (interface{}, error) {
	return MapApiSessionCertificateToRestModel(cert)
}

func MapApiSessionCertificateToRestModel(apiSessionCert *model.ApiSessionCertificate) (*rest_model.CurrentAPISessionCertificateDetail, error) {

	validFrom := strfmt.DateTime(*apiSessionCert.ValidAfter)
	validTo := strfmt.DateTime(*apiSessionCert.ValidBefore)

	ret := &rest_model.CurrentAPISessionCertificateDetail{
		BaseEntity:  BaseEntityToRestModel(apiSessionCert, CurrentApiSessionCertificateLinkFactory),
		Fingerprint: &apiSessionCert.Fingerprint,
		Subject:     &apiSessionCert.Subject,
		ValidFrom:   &validFrom,
		ValidTo:     &validTo,
		Certificate: &apiSessionCert.PEM,
	}

	return ret, nil
}
