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
	"github.com/openziti/ziti/controller/rest_server/operations/router"
)

func init() {
	r := NewFabricRouterRouter()
	env.AddRouter(r)
}

type FabricRouterRouter struct {
	BasePath string
}

func NewFabricRouterRouter() *FabricRouterRouter {
	return &FabricRouterRouter{
		BasePath: "/" + EntityNameRouter,
	}
}

func (r *FabricRouterRouter) Register(ae *env.AppEnv) {
	ae.FabricApi.RouterDeleteRouterHandler = router.DeleteRouterHandlerFunc(func(params router.DeleteRouterParams) middleware.Responder {
		return ae.IsAllowed(r.Delete, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.FabricApi.RouterDetailRouterHandler = router.DetailRouterHandlerFunc(func(params router.DetailRouterParams) middleware.Responder {
		return ae.IsAllowed(r.Detail, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.FabricApi.RouterListRoutersHandler = router.ListRoutersHandlerFunc(func(params router.ListRoutersParams) middleware.Responder {
		return ae.IsAllowed(r.ListRouters, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.FabricApi.RouterCreateRouterHandler = router.CreateRouterHandlerFunc(func(params router.CreateRouterParams) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Create(ae, rc, params) }, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.FabricApi.RouterUpdateRouterHandler = router.UpdateRouterHandlerFunc(func(params router.UpdateRouterParams) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Update(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.FabricApi.RouterPatchRouterHandler = router.PatchRouterHandlerFunc(func(params router.PatchRouterParams) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Patch(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.FabricApi.RouterListRouterTerminatorsHandler = router.ListRouterTerminatorsHandlerFunc(func(params router.ListRouterTerminatorsParams) middleware.Responder {
		return ae.IsAllowed(r.listManagementTerminators, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})
}

func (r *FabricRouterRouter) ListRouters(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler[*model.Router](ae, rc, ae.Managers.Router, FabricRouterModelMapper{}.ToApi)
}

func (r *FabricRouterRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler[*model.Router](ae, rc, ae.Managers.Router, FabricRouterModelMapper{}.ToApi)
}

func (r *FabricRouterRouter) Create(ae *env.AppEnv, rc *response.RequestContext, params router.CreateRouterParams) {
	CreateFabric(rc, FabricRouterLinkFactory, func() (string, error) {
		modelEntity := MapCreateFabricRouterToModel(params.Router)
		err := ae.Managers.Router.Create(modelEntity, rc.NewChangeContext())
		if err != nil {
			return "", err
		}
		return modelEntity.Id, nil
	})
}

func (r *FabricRouterRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ae.Managers.Router)
}

func (r *FabricRouterRouter) Update(ae *env.AppEnv, rc *response.RequestContext, params router.UpdateRouterParams) {
	Update(rc, func(id string) error {
		return ae.Managers.Router.Update(MapUpdateFabricRouterToModel(params.ID, params.Router), nil, rc.NewChangeContext())
	})
}

func (r *FabricRouterRouter) Patch(ae *env.AppEnv, rc *response.RequestContext, params router.PatchRouterParams) {
	Patch(rc, func(id string, fields fields.UpdatedFields) error {
		return ae.Managers.Router.Update(MapPatchFabricRouterToModel(params.ID, params.Router), fields.FilterMaps("tags"), rc.NewChangeContext())
	})
}

func (r *FabricRouterRouter) listManagementTerminators(ae *env.AppEnv, rc *response.RequestContext) {
	ListAssociationWithHandler[*model.Router, *model.Terminator](ae, rc, ae.Managers.Router, ae.Managers.Terminator, MapFabricTerminatorToRestEntity)
}
