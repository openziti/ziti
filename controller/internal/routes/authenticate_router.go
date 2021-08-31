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
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/mitchellh/mapstructure"
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/internal/permissions"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/controller/response"
	clientApiAuthentication "github.com/openziti/edge/rest_client_api_server/operations/authentication"
	managementApiAuthentication "github.com/openziti/edge/rest_management_api_server/operations/authentication"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/foundation/common/constants"
	"github.com/openziti/foundation/metrics"
	"github.com/openziti/foundation/util/errorz"
	"net"
	"net/http"
	"time"
)

func init() {
	r := NewAuthRouter()
	env.AddRouter(r)
}

type AuthRouter struct {
	createTimer metrics.Timer
}

func NewAuthRouter() *AuthRouter {
	return &AuthRouter{}
}

func (ro *AuthRouter) Register(ae *env.AppEnv) {
	ro.createTimer = ae.GetHostController().GetNetwork().GetMetricsRegistry().Timer("api-session.create")
	ae.ClientApi.AuthenticationAuthenticateHandler = clientApiAuthentication.AuthenticateHandlerFunc(func(params clientApiAuthentication.AuthenticateParams) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			ro.authHandler(ae, rc, params.HTTPRequest, params.Method, params.Auth)
		}, params.HTTPRequest, "", "", permissions.Always())
	})

	ae.ClientApi.AuthenticationAuthenticateMfaHandler = clientApiAuthentication.AuthenticateMfaHandlerFunc(func(params clientApiAuthentication.AuthenticateMfaParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { ro.authMfa(ae, rc, params.MfaAuth) }, params.HTTPRequest, "", "", permissions.HasOneOf(permissions.IsAuthenticated(), permissions.IsPartiallyAuthenticated()))
	})

	ae.ManagementApi.AuthenticationAuthenticateHandler = managementApiAuthentication.AuthenticateHandlerFunc(func(params managementApiAuthentication.AuthenticateParams) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			ro.authHandler(ae, rc, params.HTTPRequest, params.Method, params.Auth)
		}, params.HTTPRequest, "", "", permissions.Always())
	})

	ae.ManagementApi.AuthenticationAuthenticateMfaHandler = managementApiAuthentication.AuthenticateMfaHandlerFunc(func(params managementApiAuthentication.AuthenticateMfaParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { ro.authMfa(ae, rc, params.MfaAuth) }, params.HTTPRequest, "", "", permissions.HasOneOf(permissions.IsAuthenticated(), permissions.IsPartiallyAuthenticated()))
	})
}

func (ro *AuthRouter) authHandler(ae *env.AppEnv, rc *response.RequestContext, httpRequest *http.Request, method string, auth *rest_model.Authenticate) {
	start := time.Now()
	logger := pfxlog.Logger()
	authContext := model.NewAuthContextHttp(httpRequest, method, auth)

	identity, err := ae.Handlers.Authenticator.IsAuthorized(authContext)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	if identity == nil {
		rc.RespondWithApiError(errorz.NewUnauthorized())
		return
	}

	if identity.EnvInfo == nil {
		identity.EnvInfo = &model.EnvInfo{}
	}

	if identity.SdkInfo == nil {
		identity.SdkInfo = &model.SdkInfo{}
	}

	if dataMap := authContext.GetData(); dataMap != nil {
		shouldUpdate := false

		if envInfoInterface := dataMap["envInfo"]; envInfoInterface != nil {
			if envInfo := envInfoInterface.(map[string]interface{}); envInfo != nil {
				if err := mapstructure.Decode(envInfo, &identity.EnvInfo); err != nil {
					logger.WithError(err).Error("error processing env info")
				} else {
					shouldUpdate = true
				}

			}
		}

		if sdkInfoInterface := dataMap["sdkInfo"]; sdkInfoInterface != nil {
			if sdkInfo := sdkInfoInterface.(map[string]interface{}); sdkInfo != nil {
				if err := mapstructure.Decode(sdkInfo, &identity.SdkInfo); err != nil {
					logger.WithError(err).Error("error processing sdk info")
				} else {
					shouldUpdate = true
				}
			}
		}

		if shouldUpdate {
			if err := ae.GetHandlers().Identity.PatchInfo(identity); err != nil {
				logger.WithError(err).Errorf("failed to update sdk/env info on identity [%s] auth", identity.Id)
			}
		}
	}

	token := uuid.New().String()
	configTypes := map[string]struct{}{}

	if auth != nil {
		configTypes = ae.Handlers.ConfigType.MapConfigTypeNamesToIds(auth.ConfigTypes, identity.Id)
	}
	remoteIpStr := ""
	if remoteIp, _, err := net.SplitHostPort(rc.Request.RemoteAddr); err == nil {
		remoteIpStr = remoteIp
	}

	logger.Debugf("client %v requesting configTypes: %v", identity.Name, configTypes)
	newApiSession := &model.ApiSession{
		IdentityId:     identity.Id,
		Token:          token,
		ConfigTypes:    configTypes,
		IPAddress:      remoteIpStr,
		LastActivityAt: time.Now().UTC(),
	}

	mfa, err := ae.Handlers.Mfa.ReadByIdentityId(identity.Id)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	if mfa != nil && mfa.IsVerified {
		newApiSession.MfaRequired = true
		newApiSession.MfaComplete = false
	}

	sessionId, err := ae.Handlers.ApiSession.Create(newApiSession)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	filledApiSession, err := ae.Handlers.ApiSession.Read(sessionId)

	if err != nil {
		logger.WithField("cause", err).Error("loading session by id resulted in an error")
		rc.RespondWithApiError(errorz.NewUnauthorized())
	}

	ae.GetHandlers().PostureResponse.SetSdkInfo(identity.Id, sessionId, identity.SdkInfo)

	apiSession := MapToCurrentApiSessionRestModel(ae, filledApiSession, ae.Config.SessionTimeoutDuration())
	rc.ApiSession = filledApiSession

	//re-calc session headers as they were not set when ApiSession == NIL
	response.AddSessionHeaders(rc)

	envelope := &rest_model.CurrentAPISessionDetailEnvelope{Data: apiSession, Meta: &rest_model.Meta{}}

	rc.ResponseWriter.Header().Set(constants.ZitiSession, filledApiSession.Token)

	ro.createTimer.UpdateSince(start)

	rc.Respond(envelope, http.StatusOK)
}

func (ro *AuthRouter) authMfa(ae *env.AppEnv, rc *response.RequestContext, mfaCode *rest_model.MfaCode) {
	mfa, err := ae.Handlers.Mfa.ReadByIdentityId(rc.Identity.Id)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	if mfa == nil {
		rc.RespondWithError(apierror.NewMfaNotEnrolledError())
		return
	}

	ok, _ := ae.Handlers.Mfa.Verify(mfa, *mfaCode.Code)

	if !ok {
		rc.RespondWithError(apierror.NewInvalidMfaTokenError())
		return
	}

	if err := ae.Handlers.ApiSession.MfaCompleted(rc.ApiSession); err != nil {
		rc.RespondWithError(err)
		return
	}

	ae.Handlers.PostureResponse.SetMfaPosture(rc.Identity.Id, rc.ApiSession.Id, true)

	rc.RespondWithEmptyOk()
}
