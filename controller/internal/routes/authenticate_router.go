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
	"github.com/go-openapi/runtime/middleware"
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/mitchellh/mapstructure"
	clientApiAuthentication "github.com/openziti/edge-api/rest_client_api_server/operations/authentication"
	managementApiAuthentication "github.com/openziti/edge-api/rest_management_api_server/operations/authentication"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/metrics"
	"github.com/openziti/ziti/controller/apierror"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/internal/permissions"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/response"
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
	authContext := model.NewAuthContextHttp(httpRequest, method, auth, rc.NewChangeContext())

	authResult, err := ae.Managers.Authenticator.Authorize(authContext)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	if !authResult.IsSuccessful() {
		rc.RespondWithApiError(errorz.NewUnauthorized())
		return
	}

	rc.AuthPolicy = authResult.AuthPolicy()

	identity := authResult.Identity()
	rc.Identity = identity

	if identity.EnvInfo == nil {
		identity.EnvInfo = &model.EnvInfo{}
	}

	if identity.SdkInfo == nil {
		identity.SdkInfo = &model.SdkInfo{}
	}

	changeCtx := rc.NewChangeContext()

	if dataMap := authContext.GetData(); dataMap != nil {
		shouldUpdate := false

		if envInfoInterface := dataMap["envInfo"]; envInfoInterface != nil {
			if envInfoMap := envInfoInterface.(map[string]interface{}); envInfoMap != nil {
				var envInfo *model.EnvInfo
				if err := mapstructure.Decode(envInfoMap, &envInfo); err != nil {
					logger.WithError(err).Error("error processing env info")
				} else if !identity.EnvInfo.Equals(envInfo) {
					identity.EnvInfo = envInfo
					shouldUpdate = true
				}
			}
		}

		if sdkInfoInterface := dataMap["sdkInfo"]; sdkInfoInterface != nil {
			if sdkInfoMap := sdkInfoInterface.(map[string]interface{}); sdkInfoMap != nil {
				var sdkInfo *model.SdkInfo
				if err := mapstructure.Decode(sdkInfoMap, &sdkInfo); err != nil {
					logger.WithError(err).Error("error processing sdk info")
				} else if !identity.SdkInfo.Equals(sdkInfo) {
					identity.SdkInfo = sdkInfo
					shouldUpdate = true
				}
			}
		}

		if shouldUpdate {
			if err := ae.GetManagers().Identity.PatchInfo(identity, changeCtx); err != nil {
				logger.WithError(err).Errorf("failed to update sdk/env info on identity [%s] auth", identity.Id)
			}
		}
	}

	token := uuid.New().String()
	configTypes := map[string]struct{}{}

	if auth != nil {
		configTypes = ae.Managers.ConfigType.MapConfigTypeNamesToIds(auth.ConfigTypes, identity.Id)
	}
	remoteIpStr := ""
	if remoteIp, _, err := net.SplitHostPort(rc.Request.RemoteAddr); err == nil {
		remoteIpStr = remoteIp
	}

	logger.Debugf("client %v requesting configTypes: %v", identity.Name, configTypes)
	newApiSession := &model.ApiSession{
		IdentityId:      identity.Id,
		Token:           token,
		ConfigTypes:     configTypes,
		IPAddress:       remoteIpStr,
		AuthenticatorId: authResult.AuthenticatorId(),
		LastActivityAt:  time.Now().UTC(),
	}

	mfa, err := ae.Managers.Mfa.ReadOneByIdentityId(identity.Id)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	if mfa != nil && mfa.IsVerified {
		newApiSession.MfaRequired = true
		newApiSession.MfaComplete = false
	}

	var sessionCerts []*model.ApiSessionCertificate

	for _, cert := range authResult.SessionCerts() {
		sessionCert := model.NewApiSessionCertificate(cert)
		sessionCerts = append(sessionCerts, sessionCert)
	}

	var sessionIdHolder concurrenz.AtomicValue[string]

	ctrl, err := ae.AuthRateLimiter.RunRateLimited(func() error {
		sessionId, err := ae.Managers.ApiSession.Create(changeCtx.NewMutateContext(), newApiSession, sessionCerts)
		sessionIdHolder.Store(sessionId)
		return err
	})

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	sessionId := sessionIdHolder.Load()

	filledApiSession, err := ae.Managers.ApiSession.Read(sessionId)

	if err != nil {
		logger.WithField("cause", err).Error("loading session by id resulted in an error")
		rc.RespondWithApiError(errorz.NewUnauthorized())
		return
	}

	ae.GetManagers().PostureResponse.SetSdkInfo(identity.Id, sessionId, identity.SdkInfo)

	rc.ApiSession = filledApiSession

	env.ProcessAuthQueries(ae, rc)

	apiSession := MapToCurrentApiSessionRestModel(ae, rc, ae.Config.SessionTimeoutDuration())

	//re-calc session headers as they were not set when ApiSession == NIL
	response.AddSessionHeaders(rc)

	envelope := &rest_model.CurrentAPISessionDetailEnvelope{Data: apiSession, Meta: &rest_model.Meta{}}

	rc.ResponseWriter.Header().Set(env.ZitiSession, filledApiSession.Token)

	ro.createTimer.UpdateSince(start)

	writeOk := rc.RespondWithProducer(rc.GetProducer(), envelope, http.StatusOK)
	if writeOk {
		ctrl.Success()
	} else {
		ctrl.Timeout()
	}
}

func (ro *AuthRouter) authMfa(ae *env.AppEnv, rc *response.RequestContext, mfaCode *rest_model.MfaCode) {
	mfa, err := ae.Managers.Mfa.ReadOneByIdentityId(rc.Identity.Id)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	if mfa == nil {
		rc.RespondWithError(apierror.NewMfaNotEnrolledError())
		return
	}

	ok, _ := ae.Managers.Mfa.Verify(mfa, *mfaCode.Code, rc.NewChangeContext())

	if !ok {
		rc.RespondWithError(apierror.NewInvalidMfaTokenError())
		return
	}

	if err := ae.Managers.ApiSession.MfaCompleted(rc.ApiSession, rc.NewChangeContext()); err != nil {
		rc.RespondWithError(err)
		return
	}

	ae.Managers.PostureResponse.SetMfaPosture(rc.Identity.Id, rc.ApiSession.Id, true)

	rc.RespondWithEmptyOk()
}
