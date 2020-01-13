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
	"github.com/netfoundry/ziti-foundation/util/stringz"
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

	identity, err := ae.Handlers.Authenticator.IsAuthorized(authContext)

	if err != nil {
		rc.RequestResponder.RespondWithError(err)
		return
	}

	if identity == nil {
		rc.RequestResponder.RespondWithUnauthorizedError(rc)
		return
	}

	token := uuid.New().String()
	configTypes := ro.mapConfigTypeNamesToIds(ae, authContext.GetDataStringSlice("configTypes"))

	s := &model.ApiSession{
		IdentityId:  identity.Id,
		Token:       token,
		ConfigTypes: stringz.SliceToSet(configTypes),
	}

	sessionId, err := ae.Handlers.ApiSession.Create(s)

	if err != nil {
		rc.RequestResponder.RespondWithError(err)
		return
	}

	session, err := ae.Handlers.ApiSession.Read(sessionId)

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

func (ro *AuthRouter) mapConfigTypeNamesToIds(ae *env.AppEnv, values []string) []string {
	var result []string
	if stringz.Contains(values, "all") {
		return []string{"all"}
	}
	for _, val := range values {
		if configType, err := ae.GetHandlers().ConfigType.Read(val); err == nil && configType != nil {
			result = append(result, val)
		} else if configType, err := ae.GetHandlers().ConfigType.ReadByName(val); err == nil && configType != nil {
			result = append(result, configType.Id)
		}
	}
	return result
}
