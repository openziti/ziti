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
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/fields"
	"github.com/openziti/ziti/controller/internal/permissions"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/response"
	"github.com/openziti/ziti/controller/rest_server/operations/terminator"
)

func init() {
	r := NewFabricTerminatorRouter()
	env.AddRouter(r)
}

type FabricTerminatorRouter struct {
	BasePath string
}

func NewFabricTerminatorRouter() *FabricTerminatorRouter {
	return &FabricTerminatorRouter{
		BasePath: "/" + EntityNameTerminator,
	}
}

func (r *FabricTerminatorRouter) Register(ae *env.AppEnv) {
	ae.FabricApi.TerminatorDeleteTerminatorHandler = terminator.DeleteTerminatorHandlerFunc(func(params terminator.DeleteTerminatorParams) middleware.Responder {
		return ae.IsAllowed(r.Delete, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.FabricApi.TerminatorDetailTerminatorHandler = terminator.DetailTerminatorHandlerFunc(func(params terminator.DetailTerminatorParams) middleware.Responder {
		return ae.IsAllowed(r.Detail, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.FabricApi.TerminatorListTerminatorsHandler = terminator.ListTerminatorsHandlerFunc(func(params terminator.ListTerminatorsParams) middleware.Responder {
		return ae.IsAllowed(r.List, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.FabricApi.TerminatorUpdateTerminatorHandler = terminator.UpdateTerminatorHandlerFunc(func(params terminator.UpdateTerminatorParams) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Update(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.FabricApi.TerminatorCreateTerminatorHandler = terminator.CreateTerminatorHandlerFunc(func(params terminator.CreateTerminatorParams) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Create(ae, rc, params) }, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.FabricApi.TerminatorPatchTerminatorHandler = terminator.PatchTerminatorHandlerFunc(func(params terminator.PatchTerminatorParams) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Patch(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})
}

func (r *FabricTerminatorRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler[*model.Terminator](ae, rc, ae.Managers.Terminator, MapFabricTerminatorToRestEntity)
}

func (r *FabricTerminatorRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler[*model.Terminator](ae, rc, ae.Managers.Terminator, MapFabricTerminatorToRestEntity)
}

func (r *FabricTerminatorRouter) Create(ae *env.AppEnv, rc *response.RequestContext, params terminator.CreateTerminatorParams) {
	CreateFabric(rc, FabricTerminatorLinkFactory, func() (string, error) {
		entity := MapCreateFabricTerminatorToModel(params.Terminator)
		err := ae.Managers.Terminator.Create(entity, rc.NewChangeContext())
		if err != nil {
			return "", err
		}
		return entity.Id, nil
	})
}

func (r *FabricTerminatorRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ae.Managers.Terminator)
}

func (r *FabricTerminatorRouter) Update(ae *env.AppEnv, rc *response.RequestContext, params terminator.UpdateTerminatorParams) {
	Update(rc, func(id string) error {
		return ae.Managers.Terminator.Update(MapUpdateFabricTerminatorToModel(params.ID, params.Terminator), nil, rc.NewChangeContext())
	})
}

func (r *FabricTerminatorRouter) Patch(ae *env.AppEnv, rc *response.RequestContext, params terminator.PatchTerminatorParams) {
	Patch(rc, func(id string, fields fields.UpdatedFields) error {
		return ae.Managers.Terminator.Update(MapPatchFabricTerminatorToModel(params.ID, params.Terminator), fields.FilterMaps("tags"), rc.NewChangeContext())
	})
}
