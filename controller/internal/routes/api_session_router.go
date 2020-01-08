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
	"github.com/netfoundry/ziti-edge/controller/env"
	"github.com/netfoundry/ziti-edge/controller/internal/permissions"
	"github.com/netfoundry/ziti-edge/controller/response"
)

func init() {
	r := NewApiSessionRouter()
	env.AddRouter(r)
}

type ApiSessionRouter struct {
	BasePath string
	IdType   response.IdType
}

func NewApiSessionRouter() *ApiSessionRouter {
	return &ApiSessionRouter{
		BasePath: "/" + EntityNameApiSession,
		IdType:   response.IdTypeUuid,
	}
}

func (ir *ApiSessionRouter) Register(ae *env.AppEnv) {
	registerReadDeleteOnlyRouter(ae, ae.RootRouter, ir.BasePath, ir, permissions.IsAdmin())
}

func (ir *ApiSessionRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler(ae, rc, ae.Handlers.ApiSession, MapApiSessionToApiEntity)
}

func (ir *ApiSessionRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Handlers.ApiSession, MapApiSessionToApiEntity, ir.IdType)
}

func (ir *ApiSessionRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ir.IdType, ae.Handlers.ApiSession)
}
