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
	"errors"
	"net/http"
	"time"

	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/strfmt"
	clientCurrentApiSession "github.com/openziti/edge-api/rest_client_api_server/operations/current_api_session"
	managementCurrentApiSession "github.com/openziti/edge-api/rest_management_api_server/operations/current_api_session"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/v2/controller/apierror"
	"github.com/openziti/ziti/v2/controller/env"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/permissions"
	"github.com/openziti/ziti/v2/controller/response"
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
	//Client
	ae.ClientApi.CurrentAPISessionGetCurrentAPISessionHandler = clientCurrentApiSession.GetCurrentAPISessionHandlerFunc(func(params clientCurrentApiSession.GetCurrentAPISessionParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(router.Detail, params.HTTPRequest, "", "", permissions.HasOneOf(permissions.IsAuthenticated(), permissions.IsPartiallyAuthenticated()))
	})

	ae.ClientApi.CurrentAPISessionDeleteCurrentAPISessionHandler = clientCurrentApiSession.DeleteCurrentAPISessionHandlerFunc(func(params clientCurrentApiSession.DeleteCurrentAPISessionParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(router.Delete, params.HTTPRequest, "", "", permissions.HasOneOf(permissions.IsAuthenticated(), permissions.IsPartiallyAuthenticated()))
	})

	ae.ClientApi.CurrentAPISessionListCurrentAPISessionCertificatesHandler = clientCurrentApiSession.ListCurrentAPISessionCertificatesHandlerFunc(func(params clientCurrentApiSession.ListCurrentAPISessionCertificatesParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(router.ListCertificates, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})

	ae.ClientApi.CurrentAPISessionCreateCurrentAPISessionCertificateHandler = clientCurrentApiSession.CreateCurrentAPISessionCertificateHandlerFunc(func(params clientCurrentApiSession.CreateCurrentAPISessionCertificateParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			router.CreateCertificate(ae, rc, params)
		}, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})

	ae.ClientApi.CurrentAPISessionDetailCurrentAPISessionCertificateHandler = clientCurrentApiSession.DetailCurrentAPISessionCertificateHandlerFunc(func(params clientCurrentApiSession.DetailCurrentAPISessionCertificateParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(router.DetailCertificate, params.HTTPRequest, params.ID, "", permissions.IsAuthenticated())
	})

	ae.ClientApi.CurrentAPISessionDeleteCurrentAPISessionCertificateHandler = clientCurrentApiSession.DeleteCurrentAPISessionCertificateHandlerFunc(func(params clientCurrentApiSession.DeleteCurrentAPISessionCertificateParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(router.DeleteCertificate, params.HTTPRequest, params.ID, "", permissions.IsAuthenticated())
	})

	ae.ClientApi.CurrentAPISessionCreateTotpTokenHandler = clientCurrentApiSession.CreateTotpTokenHandlerFunc(func(params clientCurrentApiSession.CreateTotpTokenParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			router.CreateTotpToken(ae, rc, params.MfaValidation)
		}, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})

	// Session Updates
	ae.ClientApi.CurrentAPISessionListServiceUpdatesHandler = clientCurrentApiSession.ListServiceUpdatesHandlerFunc(func(params clientCurrentApiSession.ListServiceUpdatesParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(router.ListServiceUpdates, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})

	//Management
	ae.ManagementApi.CurrentAPISessionGetCurrentAPISessionHandler = managementCurrentApiSession.GetCurrentAPISessionHandlerFunc(func(params managementCurrentApiSession.GetCurrentAPISessionParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(router.Detail, params.HTTPRequest, "", "", permissions.HasOneOf(permissions.IsAuthenticated(), permissions.IsPartiallyAuthenticated()))
	})

	ae.ManagementApi.CurrentAPISessionDeleteCurrentAPISessionHandler = managementCurrentApiSession.DeleteCurrentAPISessionHandlerFunc(func(params managementCurrentApiSession.DeleteCurrentAPISessionParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(router.Delete, params.HTTPRequest, "", "", permissions.HasOneOf(permissions.IsAuthenticated(), permissions.IsPartiallyAuthenticated()))
	})

	ae.ManagementApi.CurrentAPISessionCreateTotpTokenHandler = managementCurrentApiSession.CreateTotpTokenHandlerFunc(func(params managementCurrentApiSession.CreateTotpTokenParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			router.CreateTotpToken(ae, rc, params.MfaValidation)
		}, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})
}

func (router *CurrentSessionRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	apiSession := MapToCurrentApiSessionRestModel(ae, rc, ae.GetConfig().Edge.SessionTimeoutDuration())

	rc.Respond(rest_model.CurrentAPISessionDetailEnvelope{Data: apiSession, Meta: &rest_model.Meta{}}, http.StatusOK)
}

func (router *CurrentSessionRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	apiSession, err := rc.SecurityCtx.GetApiSession()

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	if apiSession == nil {
		rc.RespondWithError(errors.New("api session is nil, expected a value"))
		return
	}

	err = ae.GetManagers().ApiSession.Delete(apiSession.Id, rc.NewChangeContext())

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

		result, err := ae.GetManagers().ApiSessionCertificate.BasePreparedList(query)
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

func (router *CurrentSessionRouter) CreateCertificate(ae *env.AppEnv, rc *response.RequestContext, params clientCurrentApiSession.CreateCurrentAPISessionCertificateParams) {
	apiSession, err := rc.SecurityCtx.GetApiSession()

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	identity, err := rc.SecurityCtx.GetIdentity()

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	responder := &ApiSessionCertificateCreateResponder{ae: ae, Responder: rc}
	CreateWithResponder(rc, responder, CurrentApiSessionCertificateLinkFactory, func() (string, error) {
		newApiSessionCert, err := ae.GetManagers().ApiSessionCertificate.CreateFromCSR(identity, apiSession, rc.HasJwtSecurityToken(), 12*time.Hour, []byte(*params.SessionCertificate.Csr), rc.NewChangeContext())

		if err != nil {
			return "", err
		}

		responder.ApiSessionCertificate = newApiSessionCert

		return newApiSessionCert.Id, nil
	})
}

func (router *CurrentSessionRouter) DetailCertificate(ae *env.AppEnv, rc *response.RequestContext) {
	apiSession, err := rc.SecurityCtx.GetApiSession()

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	certId, _ := rc.GetEntityId()
	cert, err := ae.GetManagers().ApiSessionCertificate.Read(certId)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	if cert.ApiSessionId != apiSession.Id {
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
	apiSession, err := rc.SecurityCtx.GetApiSession()

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	certId, _ := rc.GetEntityId()
	cert, err := ae.GetManagers().ApiSessionCertificate.Read(certId)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	if cert.ApiSessionId != apiSession.Id {
		rc.RespondWithNotFound()
		return
	}

	if err := ae.GetManagers().ApiSessionCertificate.Delete(certId, rc.NewChangeContext()); err != nil {
		rc.RespondWithError(err)
		return
	}

	rc.RespondWithEmptyOk()
}

func (router *CurrentSessionRouter) ListServiceUpdates(ae *env.AppEnv, rc *response.RequestContext) {
	apiSession, err := rc.SecurityCtx.GetApiSession()

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	identity, err := rc.SecurityCtx.GetIdentity()

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	lastUpdate := apiSession.CreatedAt
	if val, found := ae.IdentityRefreshMap.Get(identity.Id); found {
		lastUpdate = val
	} else if lastUpdate.Before(ae.StartupTime) {
		lastUpdate = ae.StartupTime
	}
	now := strfmt.DateTime(lastUpdate)
	data := &rest_model.CurrentAPISessionServiceUpdateList{
		LastChangeAt: &now,
	}
	rc.RespondWithOk(data, &rest_model.Meta{})
}

func (router *CurrentSessionRouter) CreateTotpToken(ae *env.AppEnv, rc *response.RequestContext, totpCode *rest_model.MfaCode) {

	identity, err := rc.SecurityCtx.GetIdentity()

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	if !rc.HasJwtSecurityToken() {
		rc.RespondWithApiError(apierror.NewInvalidBackingTokenTypeError())
		return
	}

	if totpCode == nil || totpCode.Code == nil || *totpCode.Code == "" {
		rc.RespondWithApiError(apierror.NewInvalidMfaTokenError())
		return
	}

	mfa, err := ae.GetManagers().Mfa.ReadOneByIdentityId(identity.Id)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	if mfa == nil {
		rc.RespondWithApiError(apierror.NewMfaNotEnrolledError())
		return
	}

	//totp codes only, no recovery
	ok, err := ae.GetManagers().Mfa.VerifyTOTP(mfa, *totpCode.Code)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	if !ok {
		rc.RespondWithApiError(apierror.NewInvalidMfaTokenError())
		return
	}

	apiSessionToken, err := rc.SecurityCtx.GetSecurityTokenCtx().GetVerifiedApiSessionToken()

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	tokenStr, tokenClaims, err := ae.CreateTotpTokenFromAccessClaims(ae.RootIssuer(), apiSessionToken.OidcToken.AccessClaims)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	issuedAt := strfmt.DateTime(tokenClaims.IssuedAt.Time)

	data := &rest_model.TotpToken{
		Token:    &tokenStr,
		IssuedAt: &issuedAt,
	}

	rc.RespondWithOk(data, nil)
}

type ApiSessionCertificateCreateResponder struct {
	response.Responder
	ApiSessionCertificate *model.ApiSessionCertificate
	ae                    *env.AppEnv
}

func (nsr *ApiSessionCertificateCreateResponder) RespondWithCreatedId(id string, _ rest_model.Link) {
	certString := nsr.ApiSessionCertificate.PEM

	newSessionEnvelope := &rest_model.CreateCurrentAPISessionCertificateEnvelope{
		Data: &rest_model.CurrentAPISessionCertificateCreateResponse{
			CreateLocation: rest_model.CreateLocation{
				Links: CurrentApiSessionCertificateLinkFactory.Links(nsr.ApiSessionCertificate),
				ID:    nsr.ApiSessionCertificate.Id,
			},
			Certificate: &certString,
			Cas:         string(nsr.ae.GetConfig().Edge.CaPems()),
		},
		Meta: &rest_model.Meta{},
	}

	nsr.Respond(newSessionEnvelope, http.StatusCreated)
}
