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
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/internal/permissions"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/edge/rest_server/operations/current_identity"
	"net/http"
)

func init() {
	r := NewCurrentIdentityRouter()
	env.AddRouter(r)
}

type CurrentIdentityRouter struct {
	BasePath string
}

func NewCurrentIdentityRouter() *CurrentIdentityRouter {
	return &CurrentIdentityRouter{
		BasePath: "/" + EntityNameCurrentIdentity,
	}
}

func (r *CurrentIdentityRouter) Register(ae *env.AppEnv) {
	ae.Api.CurrentIdentityGetCurrentIdentityHandler = current_identity.GetCurrentIdentityHandlerFunc(func(params current_identity.GetCurrentIdentityParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(detailCurrentUser, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})

	ae.Api.CurrentIdentityDeleteMfaHandler = current_identity.DeleteMfaHandlerFunc(func(params current_identity.DeleteMfaParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.removeMfa(ae, rc, params.Body)
		}, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})

	ae.Api.CurrentIdentityDetailMfaHandler = current_identity.DetailMfaHandlerFunc(func(params current_identity.DetailMfaParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(r.detailMfa, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})

	ae.Api.CurrentIdentityEnrollMfaHandler = current_identity.EnrollMfaHandlerFunc(func(params current_identity.EnrollMfaParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(r.createMfa, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})

	ae.Api.CurrentIdentityVerifyMfaHandler = current_identity.VerifyMfaHandlerFunc(func(params current_identity.VerifyMfaParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.verifyMfa(ae, rc, params.Body)
		}, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})

	ae.Api.CurrentIdentityDetailMfaQrCodeHandler = current_identity.DetailMfaQrCodeHandlerFunc(func(params current_identity.DetailMfaQrCodeParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(r.detailMfaQrCode, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})

	ae.Api.CurrentIdentityCreateMfaRecoveryCodesHandler = current_identity.CreateMfaRecoveryCodesHandlerFunc(func(params current_identity.CreateMfaRecoveryCodesParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.createMfaRecoveryCodes(ae, rc, params.MfaCode)
		}, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})

	ae.Api.CurrentIdentityDetailMfaRecoveryCodesHandler = current_identity.DetailMfaRecoveryCodesHandlerFunc(func(params current_identity.DetailMfaRecoveryCodesParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.detailMfaRecoveryCodes(ae, rc, params.Body)
		}, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})

	ae.Api.CurrentIdentityGetCurrentIdentityEdgeRoutersHandler = current_identity.GetCurrentIdentityEdgeRoutersHandlerFunc(func(params current_identity.GetCurrentIdentityEdgeRoutersParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.listEdgeRouters(ae, rc)
		}, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})
}

func (r *CurrentIdentityRouter) verifyMfa(ae *env.AppEnv, rc *response.RequestContext, body *rest_model.MfaCode) {
	mfa, err := ae.Handlers.Mfa.ReadByIdentityId(rc.Identity.Id)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	if mfa == nil {
		rc.RespondWithNotFound()
		return
	}

	if mfa.IsVerified {
		rc.RespondWithNotFound()
		return
	}

	ok, err := ae.Handlers.Mfa.VerifyTOTP(mfa, *body.Code)

	if err != nil {
		rc.RespondWithError(apierror.NewInvalidMfaTokenError())
		return
	}

	if ok {
		mfa.IsVerified = true
		ae.Handlers.Mfa.Update(mfa)
		rc.RespondWithEmptyOk()
		return

	}

	rc.RespondWithError(apierror.NewInvalidMfaTokenError())
}

func (r *CurrentIdentityRouter) createMfa(ae *env.AppEnv, rc *response.RequestContext) {
	mfa, err := ae.Handlers.Mfa.ReadByIdentityId(rc.Identity.Id)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	if mfa != nil {
		rc.RespondWithError(apierror.NewMfaExistsError())
		return
	}

	id, err := ae.Handlers.Mfa.CreateForIdentity(rc.Identity)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	rc.RespondWithCreatedId(id, CurrentIdentityMfaLinkFactory.SelfLink(rc.Identity))
}

func (r *CurrentIdentityRouter) detailMfa(ae *env.AppEnv, rc *response.RequestContext) {
	mfa, err := ae.Handlers.Mfa.ReadByIdentityId(rc.Identity.Id)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	if mfa == nil {
		rc.RespondWithNotFound()
		return
	}

	rc.SetEntityId(mfa.Id)
	Detail(rc, func(rc *response.RequestContext, id string) (interface{}, error) {
		return MapMfaToRestEntity(ae, rc, mfa)
	})
}

func (r *CurrentIdentityRouter) removeMfa(ae *env.AppEnv, rc *response.RequestContext, body *rest_model.MfaCode) {
	err := ae.Handlers.Mfa.DeleteForIdentity(rc.Identity, *body.Code)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	rc.RespondWithEmptyOk()
}

func (r *CurrentIdentityRouter) detailMfaQrCode(ae *env.AppEnv, rc *response.RequestContext) {
	mfa, err := ae.Handlers.Mfa.ReadByIdentityId(rc.Identity.Id)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	if mfa == nil {
		rc.RespondWithNotFound()
		return
	}

	if mfa.IsVerified {
		rc.RespondWithNotFound()
		return
	}

	png, err := ae.Handlers.Mfa.QrCodePng(mfa)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	rc.ResponseWriter.Header().Set("content-type", "image/png")
	rc.ResponseWriter.WriteHeader(http.StatusOK)
	rc.ResponseWriter.Write(png)
}

func (r *CurrentIdentityRouter) createMfaRecoveryCodes(ae *env.AppEnv, rc *response.RequestContext, body *rest_model.MfaCode) {
	mfa, err := ae.Handlers.Mfa.ReadByIdentityId(rc.Identity.Id)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	if mfa == nil {
		rc.RespondWithNotFound()
		return
	}

	if !mfa.IsVerified {
		rc.RespondWithNotFound()
		return
	}

	ok, err := ae.Handlers.Mfa.Verify(mfa, *body.Code)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	if !ok {
		rc.RespondWithError(apierror.NewInvalidMfaTokenError())
		return
	}

	if err := ae.Handlers.Mfa.RecreateRecoveryCodes(mfa); err != nil {
		rc.RespondWithError(err)
		return
	}

	rc.RespondWithEmptyOk()
}

func (r *CurrentIdentityRouter) detailMfaRecoveryCodes(ae *env.AppEnv, rc *response.RequestContext, body *rest_model.MfaCode) {
	mfa, err := ae.Handlers.Mfa.ReadByIdentityId(rc.Identity.Id)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	if mfa == nil {
		rc.RespondWithNotFound()
		return
	}

	if !mfa.IsVerified {
		rc.RespondWithNotFound()
		return
	}

	ok, _ := ae.Handlers.Mfa.VerifyTOTP(mfa, *body.Code)

	if !ok {
		rc.RespondWithError(apierror.NewInvalidMfaTokenError())
		return
	}

	data := &rest_model.DetailMfaRecoveryCodes{
		BaseEntity:    BaseEntityToRestModel(mfa, CurrentIdentityMfaLinkFactory),
		RecoveryCodes: mfa.RecoveryCodes,
	}

	rc.RespondWithOk(data, &rest_model.Meta{})
}

func (r *CurrentIdentityRouter) listEdgeRouters(ae *env.AppEnv, rc *response.RequestContext) {
	if rc.Identity.IsAdmin {
		filterTemplate := `isVerified = true`
		rc.SetEntityId(rc.Identity.Id)
		ListAssociationsWithFilter(ae, rc, filterTemplate, ae.Handlers.EdgeRouter, MapCurrentIdentityEdgeRouterToRestEntity)
	} else {
		filterTemplate := `isVerified = true and not isEmpty(from edgeRouterPolicies where anyOf(identities) = "%v")`
		rc.SetEntityId(rc.Identity.Id)
		ListAssociationsWithFilter(ae, rc, filterTemplate, ae.Handlers.EdgeRouter, MapCurrentIdentityEdgeRouterToRestEntity)
	}
}

func detailCurrentUser(ae *env.AppEnv, rc *response.RequestContext) {
	result, err := MapIdentityToRestModel(ae, rc.Identity)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	result.BaseEntity.Links = CurrentIdentityLinkFactory.Links(rc.Identity)

	rc.RespondWithOk(result, nil)
}
