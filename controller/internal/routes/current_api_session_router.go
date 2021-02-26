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
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/strfmt"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/internal/permissions"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/edge/rest_server/operations/current_api_session"
	"net/http"
	"time"
)

func init() {
	r := NewCurrentSessionRouter()
	env.AddRouter(r)
}

type CurrentSessionRouter struct {
}

func NewCurrentSessionRouter() *CurrentSessionRouter {
	return &CurrentSessionRouter{}
}

func (router *CurrentSessionRouter) Register(ae *env.AppEnv) {
	ae.Api.CurrentAPISessionGetCurrentAPISessionHandler = current_api_session.GetCurrentAPISessionHandlerFunc(func(params current_api_session.GetCurrentAPISessionParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(router.Detail, params.HTTPRequest, "", "", permissions.HasOneOf(permissions.IsAuthenticated(), permissions.IsPartiallyAuthenticated()))
	})

	ae.Api.CurrentAPISessionDeleteCurrentAPISessionHandler = current_api_session.DeleteCurrentAPISessionHandlerFunc(func(params current_api_session.DeleteCurrentAPISessionParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(router.Delete, params.HTTPRequest, "", "", permissions.HasOneOf(permissions.IsAuthenticated(), permissions.IsPartiallyAuthenticated()))
	})

	ae.Api.CurrentAPISessionListCurrentAPISessionCertificatesHandler = current_api_session.ListCurrentAPISessionCertificatesHandlerFunc(func(params current_api_session.ListCurrentAPISessionCertificatesParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(router.ListCertificates, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})

	ae.Api.CurrentAPISessionCreateCurrentAPISessionCertificateHandler = current_api_session.CreateCurrentAPISessionCertificateHandlerFunc(func(params current_api_session.CreateCurrentAPISessionCertificateParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			router.CreateCertificate(ae, rc, params)
		}, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})

	ae.Api.CurrentAPISessionDetailCurrentAPISessionCertificateHandler = current_api_session.DetailCurrentAPISessionCertificateHandlerFunc(func(params current_api_session.DetailCurrentAPISessionCertificateParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(router.DetailCertificate, params.HTTPRequest, params.ID, "", permissions.IsAuthenticated())
	})

	ae.Api.CurrentAPISessionDeleteCurrentAPISessionCertificateHandler = current_api_session.DeleteCurrentAPISessionCertificateHandlerFunc(func(params current_api_session.DeleteCurrentAPISessionCertificateParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(router.DeleteCertificate, params.HTTPRequest, params.ID, "", permissions.IsAuthenticated())
	})

	// Session Updates
	ae.Api.CurrentAPISessionListServiceUpdatesHandler = current_api_session.ListServiceUpdatesHandlerFunc(func(params current_api_session.ListServiceUpdatesParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(router.ListServiceUpdates, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})
}

func (router *CurrentSessionRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	apiSession := MapToCurrentApiSessionRestModel(rc.ApiSession, ae.Config.SessionTimeoutDuration())
	rc.Respond(rest_model.CurrentAPISessionDetailEnvelope{Data: apiSession, Meta: &rest_model.Meta{}}, http.StatusOK)
}

func (router *CurrentSessionRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	err := ae.GetHandlers().ApiSession.Delete(rc.ApiSession.Id)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	rc.RespondWithEmptyOk()
}

func (router *CurrentSessionRouter) ListCertificates(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithEnvelopeFactory(rc, defaultToListEnvelope, func(rc *response.RequestContext, queryOptions *PublicQueryOptions) (*QueryResult, error) {
		query, err := queryOptions.getFullQuery(ae.GetStores().ApiSessionCertificate)
		if err != nil {
			return nil, err
		}

		result, err := ae.GetHandlers().ApiSessionCertificate.BasePreparedList(query)
		if err != nil {
			return nil, err
		}

		apiEntities, err := modelToApi(ae, rc, MapApiSessionCertificateToRestEntity, result.GetEntities())
		if err != nil {
			return nil, err
		}

		return NewQueryResult(apiEntities, result.GetMetaData()), nil
	})
}

func (router *CurrentSessionRouter) CreateCertificate(ae *env.AppEnv, rc *response.RequestContext, params current_api_session.CreateCurrentAPISessionCertificateParams) {
	responder := &ApiSessionCertificateCreateResponder{ae: ae, Responder: rc}
	CreateWithResponder(rc, responder, CurrentApiSessionCertificateLinkFactory, func() (string, error) {
		return ae.GetHandlers().ApiSessionCertificate.CreateFromCSR(rc.ApiSession.Id, 12*time.Hour, []byte(*params.SessionCertificate.Csr))
	})
}

func (router *CurrentSessionRouter) DetailCertificate(ae *env.AppEnv, rc *response.RequestContext) {
	certId, _ := rc.GetEntityId()
	cert, err := ae.GetHandlers().ApiSessionCertificate.Read(certId)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	if cert.ApiSessionId != rc.ApiSession.Id {
		rc.RespondWithNotFound()
		return
	}

	entity, err := MapApiSessionCertificateToRestEntity(ae, rc, cert)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	rc.RespondWithOk(entity, nil)
}

func (router *CurrentSessionRouter) DeleteCertificate(ae *env.AppEnv, rc *response.RequestContext) {
	certId, _ := rc.GetEntityId()
	cert, err := ae.GetHandlers().ApiSessionCertificate.Read(certId)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	if cert.ApiSessionId != rc.ApiSession.Id {
		rc.RespondWithNotFound()
		return
	}

	if err := ae.GetHandlers().ApiSessionCertificate.Delete(certId); err != nil {
		rc.RespondWithError(err)
		return
	}

	rc.RespondWithEmptyOk()
}

func (router *CurrentSessionRouter) ListServiceUpdates(ae *env.AppEnv, rc *response.RequestContext) {
	lastUpdate := rc.ApiSession.CreatedAt
	if val, found := ae.IdentityRefreshMap.Get(rc.Identity.Id); found {
		lastUpdate = val.(time.Time)
	} else if lastUpdate.Before(ae.StartupTime) {
		lastUpdate = ae.StartupTime
	}
	now := strfmt.DateTime(lastUpdate)
	data := &rest_model.CurrentAPISessionServiceUpdateList{
		LastChangeAt: &now,
	}
	rc.RespondWithOk(data, &rest_model.Meta{})
}

type ApiSessionCertificateCreateResponder struct {
	response.Responder
	ae *env.AppEnv
}

func (nsr *ApiSessionCertificateCreateResponder) RespondWithCreatedId(id string, link rest_model.Link) {
	sessionCert, _ := nsr.ae.GetHandlers().ApiSessionCertificate.Read(id)
	certString := string(sessionCert.PEM)

	newSessionEnvelope := &rest_model.CreateCurrentAPISessionCertificateEnvelope{
		Data: &rest_model.CurrentAPISessionCertificateCreateResponse{
			CreateLocation: rest_model.CreateLocation{
				Links: CurrentApiSessionCertificateLinkFactory.Links(sessionCert),
				ID:    sessionCert.Id,
			},
			Certificate: &certString,
			Cas:         string(nsr.ae.Config.CaPems()),
		},
		Meta: &rest_model.Meta{},
	}

	nsr.Respond(newSessionEnvelope, http.StatusCreated)
}
