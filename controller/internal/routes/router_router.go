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
	"github.com/openziti/edge/rest_management_api_server/operations/router"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/fabric/controller/api"
)

func init() {
	r := NewTransitRouterRouter()
	env.AddRouter(r)
}

type TransitRouterRouter struct {
	BasePath string
}

func NewTransitRouterRouter() *TransitRouterRouter {
	return &TransitRouterRouter{
		BasePath: "/" + EntityNameTransitRouter,
	}
}

func (r *TransitRouterRouter) Register(ae *env.AppEnv) {
	//Router
	ae.ManagementApi.RouterDeleteRouterHandler = router.DeleteRouterHandlerFunc(func(params router.DeleteRouterParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Delete, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.RouterDetailRouterHandler = router.DetailRouterHandlerFunc(func(params router.DetailRouterParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Detail, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.RouterListRoutersHandler = router.ListRoutersHandlerFunc(func(params router.ListRoutersParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.List, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.RouterUpdateRouterHandler = router.UpdateRouterHandlerFunc(func(params router.UpdateRouterParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Update(ae, rc, params.ID, params.Router) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.RouterCreateRouterHandler = router.CreateRouterHandlerFunc(func(params router.CreateRouterParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Create(ae, rc, params.Router) }, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.RouterPatchRouterHandler = router.PatchRouterHandlerFunc(func(params router.PatchRouterParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Patch(ae, rc, params.ID, params.Router) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	//Transit Router (deprecated)
	ae.ManagementApi.RouterDeleteTransitRouterHandler = router.DeleteTransitRouterHandlerFunc(func(params router.DeleteTransitRouterParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Delete, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.RouterDetailTransitRouterHandler = router.DetailTransitRouterHandlerFunc(func(params router.DetailTransitRouterParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Detail, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.RouterListTransitRoutersHandler = router.ListTransitRoutersHandlerFunc(func(params router.ListTransitRoutersParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.List, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.RouterUpdateTransitRouterHandler = router.UpdateTransitRouterHandlerFunc(func(params router.UpdateTransitRouterParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Update(ae, rc, params.ID, params.Router) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.RouterCreateTransitRouterHandler = router.CreateTransitRouterHandlerFunc(func(params router.CreateTransitRouterParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Create(ae, rc, params.Router) }, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.RouterPatchTransitRouterHandler = router.PatchTransitRouterHandlerFunc(func(params router.PatchTransitRouterParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Patch(ae, rc, params.ID, params.Router) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})
}

func (r *TransitRouterRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler(ae, rc, ae.Managers.TransitRouter, MapTransitRouterToRestEntity)
}

func (r *TransitRouterRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Managers.TransitRouter, MapTransitRouterToRestEntity)
}

func (r *TransitRouterRouter) Create(ae *env.AppEnv, rc *response.RequestContext, router *rest_model.RouterCreate) {
	Create(rc, rc, TransitRouterLinkFactory, func() (string, error) {
		return ae.Managers.TransitRouter.Create(MapCreateRouterToModel(router))
	})
}

func (r *TransitRouterRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ae.Managers.TransitRouter)
}

func (r *TransitRouterRouter) Update(ae *env.AppEnv, rc *response.RequestContext, routerId string, router *rest_model.RouterUpdate) {
	Update(rc, func(id string) error {
		return ae.Managers.TransitRouter.Update(MapUpdateTransitRouterToModel(routerId, router), false)
	})
}

func (r *TransitRouterRouter) Patch(ae *env.AppEnv, rc *response.RequestContext, routerId string, router *rest_model.RouterPatch) {
	Patch(rc, func(id string, fields api.JsonFields) error {
		return ae.Managers.TransitRouter.Patch(MapPatchTransitRouterToModel(routerId, router), fields.ConcatNestedNames().FilterMaps("tags"), false)
	})
}
