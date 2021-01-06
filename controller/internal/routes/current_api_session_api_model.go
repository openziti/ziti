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

func MapToCurrentApiSessionRestModel(s *model.ApiSession, sessionTimeout time.Duration) *rest_model.CurrentAPISessionDetail {
	expiresAt := strfmt.DateTime(s.UpdatedAt.Add(sessionTimeout))
	expirationSeconds := int64(s.ExpirationDuration.Seconds())

	authQueries := rest_model.AuthQueryList{}

	if s.MfaRequired && !s.MfaComplete {
		authQueries = append(authQueries, newAuthCheckZitiMfa())
	}

	apiSession := &rest_model.CurrentAPISessionDetail{
		APISessionDetail: rest_model.APISessionDetail{
			BaseEntity:  BaseEntityToRestModel(s, CurrentApiSessionLinkFactory),
			ConfigTypes: stringz.SetToSlice(s.ConfigTypes),
			Identity:    ToEntityRef(s.Identity.Name, s.Identity, IdentityLinkFactory),
			IdentityID:  &s.IdentityId,
			IPAddress:   &s.IPAddress,
			Token:       &s.Token,
			AuthQueries: authQueries,
		},
		ExpiresAt:         &expiresAt,
		ExpirationSeconds: &expirationSeconds,
	}

	return apiSession
}

func MapApiSessionCertificateToRestEntity(appEnv *env.AppEnv, context *response.RequestContext, e models.Entity) (interface{}, error) {
	i, ok := e.(*model.ApiSessionCertificate)

	if !ok {
		err := fmt.Errorf("entity is not an API Session Certificate \"%s\"", e.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	al, err := MapApiSessionCertificateToRestModel(i)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return al, nil
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
