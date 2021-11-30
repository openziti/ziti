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
	"github.com/openziti/edge/rest_management_api_server/operations/terminator"
	"github.com/openziti/fabric/controller/api"
)

func init() {
	r := NewTerminatorRouter()
	env.AddRouter(r)
}

type TerminatorRouter struct {
	BasePath string
}

func NewTerminatorRouter() *TerminatorRouter {
	return &TerminatorRouter{
		BasePath: "/" + EntityNameTerminator,
	}
}

func (r *TerminatorRouter) Register(ae *env.AppEnv) {
	ae.ManagementApi.TerminatorDeleteTerminatorHandler = terminator.DeleteTerminatorHandlerFunc(func(params terminator.DeleteTerminatorParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Delete, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.TerminatorDetailTerminatorHandler = terminator.DetailTerminatorHandlerFunc(func(params terminator.DetailTerminatorParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Detail, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.TerminatorListTerminatorsHandler = terminator.ListTerminatorsHandlerFunc(func(params terminator.ListTerminatorsParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.List, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.TerminatorUpdateTerminatorHandler = terminator.UpdateTerminatorHandlerFunc(func(params terminator.UpdateTerminatorParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Update(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.TerminatorCreateTerminatorHandler = terminator.CreateTerminatorHandlerFunc(func(params terminator.CreateTerminatorParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Create(ae, rc, params) }, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.TerminatorPatchTerminatorHandler = terminator.PatchTerminatorHandlerFunc(func(params terminator.PatchTerminatorParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Patch(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})
}

func (r *TerminatorRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler(ae, rc, ae.Handlers.Terminator, MapTerminatorToRestEntity)
}

func (r *TerminatorRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Handlers.Terminator, MapTerminatorToRestEntity)
}

func (r *TerminatorRouter) Create(ae *env.AppEnv, rc *response.RequestContext, params terminator.CreateTerminatorParams) {
	Create(rc, rc, TerminatorLinkFactory, func() (string, error) {
		return ae.Handlers.Terminator.Create(MapCreateTerminatorToModel(params.Terminator))
	})
}

func (r *TerminatorRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ae.Handlers.Terminator)
}

func (r *TerminatorRouter) Update(ae *env.AppEnv, rc *response.RequestContext, params terminator.UpdateTerminatorParams) {
	Update(rc, func(id string) error {
		return ae.Handlers.Terminator.Update(MapUpdateTerminatorToModel(params.ID, params.Terminator))
	})
}

func (r *TerminatorRouter) Patch(ae *env.AppEnv, rc *response.RequestContext, params terminator.PatchTerminatorParams) {
	Patch(rc, func(id string, fields api.JsonFields) error {
		return ae.Handlers.Terminator.Patch(MapPatchTerminatorToModel(params.ID, params.Terminator), fields.FilterMaps("tags"))
	})
}
