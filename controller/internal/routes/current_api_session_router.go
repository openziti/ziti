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
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/internal/permissions"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/edge/rest_server/operations/current_api_session"
	"net/http"
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
		return ae.IsAllowed(router.Detail, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})

	ae.Api.CurrentAPISessionDeleteCurrentAPISessionHandler = current_api_session.DeleteCurrentAPISessionHandlerFunc(func(params current_api_session.DeleteCurrentAPISessionParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(router.Delete, params.HTTPRequest, "", "", permissions.IsAuthenticated())
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
