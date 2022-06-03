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
	"github.com/openziti/edge/rest_management_api_server/operations/api_session"
)

func init() {
	r := NewApiSessionRouter()
	env.AddRouter(r)
}

type ApiSessionHandler struct {
	BasePath string
}

func NewApiSessionRouter() *ApiSessionHandler {
	return &ApiSessionHandler{
		BasePath: "/" + EntityNameApiSession,
	}
}

func (ir *ApiSessionHandler) Register(ae *env.AppEnv) {
	ae.ManagementApi.APISessionDeleteAPISessionsHandler = api_session.DeleteAPISessionsHandlerFunc(func(params api_session.DeleteAPISessionsParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(ir.Delete, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.APISessionDetailAPISessionsHandler = api_session.DetailAPISessionsHandlerFunc(func(params api_session.DetailAPISessionsParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(ir.Detail, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.APISessionListAPISessionsHandler = api_session.ListAPISessionsHandlerFunc(func(params api_session.ListAPISessionsParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(ir.List, params.HTTPRequest, "", "", permissions.IsAdmin())
	})
}

func (ir *ApiSessionHandler) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler(ae, rc, ae.Managers.ApiSession, MapApiSessionToRestInterface)
}

func (ir *ApiSessionHandler) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Managers.ApiSession, MapApiSessionToRestInterface)
}

func (ir *ApiSessionHandler) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ae.Managers.ApiSession)
}
