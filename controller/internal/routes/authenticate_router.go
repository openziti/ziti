/*
	Copyright 2019 Netfoundry, Inc.

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
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/env"
	"github.com/netfoundry/ziti-edge/controller/internal/permissions"
	"github.com/netfoundry/ziti-edge/controller/model"
	"github.com/netfoundry/ziti-edge/controller/response"
	"net/http"
	"time"
)

func init() {
	r := NewAuthRouter()
	env.AddRouter(r)
}

type AuthRouter struct {
}

func NewAuthRouter() *AuthRouter {
	return &AuthRouter{}
}

func (ro *AuthRouter) Register(ae *env.AppEnv) {
	authHandler := ae.WrapHandler(ro.authHandler, permissions.Always())

	ae.RootRouter.HandleFunc("/authenticate", authHandler).Methods("POST")
	ae.RootRouter.HandleFunc("/authenticate/", authHandler).Methods("POST")
}

func (ro *AuthRouter) authHandler(ae *env.AppEnv, rc *response.RequestContext) {
	authContext := &model.AuthContextHttp{}
	err := authContext.FillFromHttpRequest(rc.Request)

	if err != nil {
		rc.RequestResponder.RespondWithError(err)
		return
	}

	identity, err := ae.Handlers.Authenticator.HandleIsAuthorized(authContext)

	if err != nil {
		rc.RequestResponder.RespondWithError(err)
		return
	}

	if identity == nil {
		rc.RequestResponder.RespondWithUnauthorizedError(rc)
		return
	}

	token := uuid.New().String()

	s := &model.ApiSession{
		IdentityId: identity.Id,
		Token:      token,
	}

	sessionId, err := ae.Handlers.ApiSession.HandleCreate(s)

	if err != nil {
		rc.RequestResponder.RespondWithError(err)
		return
	}

	session, err := ae.Handlers.ApiSession.HandleRead(sessionId)

	if err != nil {
		pfxlog.Logger().WithField("cause", err).Error("loading session by id resulted in an error")
		rc.RequestResponder.RespondWithUnauthorizedError(rc)
	}

	currentSession, err := RenderCurrentSessionApiListEntity(session, ae.Config.SessionTimeoutDuration())

	if err != nil {
		rc.RequestResponder.RespondWithError(err)
		return
	}

	expiration := time.Now().Add(365 * 24 * time.Hour)
	cookie := http.Cookie{Name: ae.AuthCookieName, Value: token, Expires: expiration}
	rc.ResponseWriter.Header().Set(ae.AuthHeaderName, session.Token)
	http.SetCookie(rc.ResponseWriter, &cookie)

	rc.RequestResponder.RespondWithOk(currentSession, nil)
}
