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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/internal/permissions"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/controller/response"
	clientCurrentIdentity "github.com/openziti/edge/rest_client_api_server/operations/current_identity"
	managementCurrentIdentity "github.com/openziti/edge/rest_management_api_server/operations/current_identity"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/foundation/util/errorz"
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
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
	//Client
	ae.ClientApi.CurrentIdentityGetCurrentIdentityHandler = clientCurrentIdentity.GetCurrentIdentityHandlerFunc(func(params clientCurrentIdentity.GetCurrentIdentityParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(detailCurrentUser, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})

	ae.ClientApi.CurrentIdentityDeleteMfaHandler = clientCurrentIdentity.DeleteMfaHandlerFunc(func(params clientCurrentIdentity.DeleteMfaParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.removeMfa(ae, rc, params.MfaValidation, params.MfaValidationCode)
		}, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})

	ae.ClientApi.CurrentIdentityDetailMfaHandler = clientCurrentIdentity.DetailMfaHandlerFunc(func(params clientCurrentIdentity.DetailMfaParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(r.detailMfa, params.HTTPRequest, "", "", permissions.NewRequireOne(permissions.AuthenticatedPermission, permissions.PartiallyAuthenticatePermission))
	})

	ae.ClientApi.CurrentIdentityEnrollMfaHandler = clientCurrentIdentity.EnrollMfaHandlerFunc(func(params clientCurrentIdentity.EnrollMfaParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(r.createMfa, params.HTTPRequest, "", "", permissions.NewRequireOne(permissions.AuthenticatedPermission, permissions.PartiallyAuthenticatePermission))
	})

	ae.ClientApi.CurrentIdentityVerifyMfaHandler = clientCurrentIdentity.VerifyMfaHandlerFunc(func(params clientCurrentIdentity.VerifyMfaParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.verifyMfa(ae, rc, params.MfaValidation)
		}, params.HTTPRequest, "", "", permissions.NewRequireOne(permissions.AuthenticatedPermission, permissions.PartiallyAuthenticatePermission))
	})

	ae.ClientApi.CurrentIdentityDetailMfaQrCodeHandler = clientCurrentIdentity.DetailMfaQrCodeHandlerFunc(func(params clientCurrentIdentity.DetailMfaQrCodeParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(r.detailMfaQrCode, params.HTTPRequest, "", "", permissions.NewRequireOne(permissions.AuthenticatedPermission, permissions.PartiallyAuthenticatePermission))
	})

	ae.ClientApi.CurrentIdentityCreateMfaRecoveryCodesHandler = clientCurrentIdentity.CreateMfaRecoveryCodesHandlerFunc(func(params clientCurrentIdentity.CreateMfaRecoveryCodesParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.createMfaRecoveryCodes(ae, rc, params.MfaValidation)
		}, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})

	ae.ClientApi.CurrentIdentityDetailMfaRecoveryCodesHandler = clientCurrentIdentity.DetailMfaRecoveryCodesHandlerFunc(func(params clientCurrentIdentity.DetailMfaRecoveryCodesParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.detailMfaRecoveryCodes(ae, rc, params.MfaValidation, params.MfaValidationCode)
		}, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})

	ae.ClientApi.CurrentIdentityGetCurrentIdentityEdgeRoutersHandler = clientCurrentIdentity.GetCurrentIdentityEdgeRoutersHandlerFunc(func(params clientCurrentIdentity.GetCurrentIdentityEdgeRoutersParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.listEdgeRouters(ae, rc)
		}, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})

	//Management
	ae.ManagementApi.CurrentIdentityGetCurrentIdentityHandler = managementCurrentIdentity.GetCurrentIdentityHandlerFunc(func(params managementCurrentIdentity.GetCurrentIdentityParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(detailCurrentUser, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})

	ae.ManagementApi.CurrentIdentityDeleteMfaHandler = managementCurrentIdentity.DeleteMfaHandlerFunc(func(params managementCurrentIdentity.DeleteMfaParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.removeMfa(ae, rc, params.MfaValidation, params.MfaValidationCode)
		}, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})

	ae.ManagementApi.CurrentIdentityDetailMfaHandler = managementCurrentIdentity.DetailMfaHandlerFunc(func(params managementCurrentIdentity.DetailMfaParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(r.detailMfa, params.HTTPRequest, "", "", permissions.NewRequireOne(permissions.AuthenticatedPermission, permissions.PartiallyAuthenticatePermission))
	})

	ae.ManagementApi.CurrentIdentityEnrollMfaHandler = managementCurrentIdentity.EnrollMfaHandlerFunc(func(params managementCurrentIdentity.EnrollMfaParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(r.createMfa, params.HTTPRequest, "", "", permissions.NewRequireOne(permissions.AuthenticatedPermission, permissions.PartiallyAuthenticatePermission))
	})

	ae.ManagementApi.CurrentIdentityVerifyMfaHandler = managementCurrentIdentity.VerifyMfaHandlerFunc(func(params managementCurrentIdentity.VerifyMfaParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.verifyMfa(ae, rc, params.MfaValidation)
		}, params.HTTPRequest, "", "", permissions.NewRequireOne(permissions.AuthenticatedPermission, permissions.PartiallyAuthenticatePermission))
	})

	ae.ManagementApi.CurrentIdentityDetailMfaQrCodeHandler = managementCurrentIdentity.DetailMfaQrCodeHandlerFunc(func(params managementCurrentIdentity.DetailMfaQrCodeParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(r.detailMfaQrCode, params.HTTPRequest, "", "", permissions.NewRequireOne(permissions.AuthenticatedPermission, permissions.PartiallyAuthenticatePermission))
	})

	ae.ManagementApi.CurrentIdentityCreateMfaRecoveryCodesHandler = managementCurrentIdentity.CreateMfaRecoveryCodesHandlerFunc(func(params managementCurrentIdentity.CreateMfaRecoveryCodesParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.createMfaRecoveryCodes(ae, rc, params.MfaValidation)
		}, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})

	ae.ManagementApi.CurrentIdentityDetailMfaRecoveryCodesHandler = managementCurrentIdentity.DetailMfaRecoveryCodesHandlerFunc(func(params managementCurrentIdentity.DetailMfaRecoveryCodesParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.detailMfaRecoveryCodes(ae, rc, params.MfaValidation, params.MfaValidationCode)
		}, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})
}

func (r *CurrentIdentityRouter) verifyMfa(ae *env.AppEnv, rc *response.RequestContext, body *rest_model.MfaCode) {
	mfa, err := ae.Managers.Mfa.ReadByIdentityId(rc.Identity.Id)

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

	ok, err := ae.Managers.Mfa.VerifyTOTP(mfa, *body.Code)

	if err != nil {
		rc.RespondWithError(apierror.NewInvalidMfaTokenError())
		return
	}

	if ok {
		mfa.IsVerified = true
		if err := ae.Managers.Mfa.Update(mfa); err != nil {
			pfxlog.Logger().Errorf("could not update MFA with new MFA status: %v", err)
			rc.RespondWithApiError(errorz.NewUnhandled(errors.New("could not update MFA status")))
			return
		}

		rc.ApiSession.MfaComplete = true
		rc.ApiSession.MfaRequired = true

		if err := ae.Managers.ApiSession.UpdateWithFieldChecker(rc.ApiSession, boltz.MapFieldChecker{persistence.FieldApiSessionMfaComplete: struct{}{}, persistence.FieldApiSessionMfaRequired: struct{}{}}); err != nil {
			pfxlog.Logger().Errorf("could not update API Session with new MFA status: %v", err)
		}

		ae.Managers.PostureResponse.SetMfaPosture(rc.Identity.Id, rc.ApiSession.Id, true)

		rc.RespondWithEmptyOk()
		return

	}

	rc.RespondWithError(apierror.NewInvalidMfaTokenError())
}

func (r *CurrentIdentityRouter) createMfa(ae *env.AppEnv, rc *response.RequestContext) {
	mfa, err := ae.Managers.Mfa.ReadByIdentityId(rc.Identity.Id)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	if mfa != nil {
		rc.RespondWithError(apierror.NewMfaExistsError())
		return
	}

	id, err := ae.Managers.Mfa.CreateForIdentity(rc.Identity)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	rc.RespondWithCreatedId(id, CurrentIdentityMfaLinkFactory.SelfLink(rc.Identity))
}

func (r *CurrentIdentityRouter) detailMfa(ae *env.AppEnv, rc *response.RequestContext) {
	mfa, err := ae.Managers.Mfa.ReadByIdentityId(rc.Identity.Id)

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

func (r *CurrentIdentityRouter) removeMfa(ae *env.AppEnv, rc *response.RequestContext, mfaValidationBody *rest_model.MfaCode, mfaCodeHeader *string) {
	code := ""

	if mfaValidationBody != nil && mfaValidationBody.Code != nil {
		code = *mfaValidationBody.Code
	} else if mfaCodeHeader != nil {
		code = *mfaCodeHeader
	}

	err := ae.Managers.Mfa.DeleteForIdentity(rc.Identity, code)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	ae.Managers.PostureResponse.SetMfaPostureForIdentity(rc.Identity.Id, false)

	rc.RespondWithEmptyOk()
}

func (r *CurrentIdentityRouter) detailMfaQrCode(ae *env.AppEnv, rc *response.RequestContext) {
	mfa, err := ae.Managers.Mfa.ReadByIdentityId(rc.Identity.Id)

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

	png, err := ae.Managers.Mfa.QrCodePng(mfa)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	rc.ResponseWriter.Header().Set("content-type", "image/png")
	rc.ResponseWriter.WriteHeader(http.StatusOK)
	_, _ = rc.ResponseWriter.Write(png)
}

func (r *CurrentIdentityRouter) createMfaRecoveryCodes(ae *env.AppEnv, rc *response.RequestContext, body *rest_model.MfaCode) {
	mfa, err := ae.Managers.Mfa.ReadByIdentityId(rc.Identity.Id)

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

	ok, _ := ae.Managers.Mfa.Verify(mfa, *body.Code)

	if !ok {
		rc.RespondWithError(apierror.NewInvalidMfaTokenError())
		return
	}

	if err := ae.Managers.Mfa.RecreateRecoveryCodes(mfa); err != nil {
		rc.RespondWithError(err)
		return
	}

	rc.RespondWithEmptyOk()
}

func (r *CurrentIdentityRouter) detailMfaRecoveryCodes(ae *env.AppEnv, rc *response.RequestContext, mfaValidationBody *rest_model.MfaCode, mfaCodeHeader *string) {
	mfa, err := ae.Managers.Mfa.ReadByIdentityId(rc.Identity.Id)

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

	code := ""

	if mfaValidationBody != nil && mfaValidationBody.Code != nil {
		code = *mfaValidationBody.Code
	} else if mfaCodeHeader != nil {
		code = *mfaCodeHeader
	}

	if code == "" {
		rc.RespondWithError(apierror.NewInvalidMfaTokenError())
		return
	}

	ok, _ := ae.Managers.Mfa.VerifyTOTP(mfa, code)

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
		ListAssociationsWithFilter(ae, rc, filterTemplate, ae.Managers.EdgeRouter, MapCurrentIdentityEdgeRouterToRestEntity)
	} else {
		filterTemplate := `isVerified = true and not isEmpty(from edgeRouterPolicies where anyOf(identities) = "%v")`
		rc.SetEntityId(rc.Identity.Id)
		ListAssociationsWithFilter(ae, rc, filterTemplate, ae.Managers.EdgeRouter, MapCurrentIdentityEdgeRouterToRestEntity)
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
