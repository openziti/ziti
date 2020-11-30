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
	"github.com/openziti/edge/rest_server/operations/informational"
)

func init() {
	r := NewProtocolRouter()
	env.AddRouter(r)
}

type ProtocolRouter struct {
	BasePath string
}

func NewProtocolRouter() *ProtocolRouter {
	return &ProtocolRouter{
		BasePath: "/Protocol",
	}
}

func (router *ProtocolRouter) Register(ae *env.AppEnv) {
	ae.Api.InformationalListProtocolsHandler = informational.ListProtocolsHandlerFunc(func(params informational.ListProtocolsParams) middleware.Responder {
		return ae.IsAllowed(router.List, params.HTTPRequest, "", "", permissions.Always())
	})
}

func (router *ProtocolRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	data := rest_model.ListProtocols{
		"https": rest_model.Protocol{
			Address: &ae.Config.Api.Advertise,
		},
	}
	rc.RespondWithOk(data, &rest_model.Meta{})
}
