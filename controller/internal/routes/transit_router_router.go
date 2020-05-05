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
	"github.com/openziti/edge/rest_server/operations/transit_router"
)

func init() {
	r := NewTransitRouterRouter()
	env.AddRouter(r)
}

type TransitRouterRouter struct {
	BasePath string
	IdType   response.IdType
}

func NewTransitRouterRouter() *TransitRouterRouter {
	return &TransitRouterRouter{
		BasePath: "/" + EntityNameTransitRouter,
		IdType:   response.IdTypeUuid,
	}
}

func (r *TransitRouterRouter) Register(ae *env.AppEnv) {
	ae.Api.TransitRouterDeleteTransitRouterHandler = transit_router.DeleteTransitRouterHandlerFunc(func(params transit_router.DeleteTransitRouterParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Delete, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.Api.TransitRouterDetailTransitRouterHandler = transit_router.DetailTransitRouterHandlerFunc(func(params transit_router.DetailTransitRouterParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Detail, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.Api.TransitRouterListTransitRoutersHandler = transit_router.ListTransitRoutersHandlerFunc(func(params transit_router.ListTransitRoutersParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.List, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.Api.TransitRouterUpdateTransitRouterHandler = transit_router.UpdateTransitRouterHandlerFunc(func(params transit_router.UpdateTransitRouterParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Update(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.Api.TransitRouterCreateTransitRouterHandler = transit_router.CreateTransitRouterHandlerFunc(func(params transit_router.CreateTransitRouterParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Create(ae, rc, params) }, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.Api.TransitRouterPatchTransitRouterHandler = transit_router.PatchTransitRouterHandlerFunc(func(params transit_router.PatchTransitRouterParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Patch(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})
}

func (r *TransitRouterRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler(ae, rc, ae.Handlers.TransitRouter, MapTransitRouterToRestEntity)
}

func (r *TransitRouterRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Handlers.TransitRouter, MapTransitRouterToRestEntity)
}

func (r *TransitRouterRouter) Create(ae *env.AppEnv, rc *response.RequestContext, params transit_router.CreateTransitRouterParams) {
	Create(rc, rc, TransitRouterLinkFactory, func() (string, error) {
		return ae.Handlers.TransitRouter.Create(MapCreateTransitRouterToModel(params.Body))
	})
}

func (r *TransitRouterRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ae.Handlers.TransitRouter)
}

func (r *TransitRouterRouter) Update(ae *env.AppEnv, rc *response.RequestContext, params transit_router.UpdateTransitRouterParams) {
	Update(rc, func(id string) error {
		return ae.Handlers.TransitRouter.Update(MapUpdateTransitRouterToModel(params.ID, params.Body), false)
	})
}

func (r *TransitRouterRouter) Patch(ae *env.AppEnv, rc *response.RequestContext, params transit_router.PatchTransitRouterParams) {
	Patch(rc, func(id string, fields JsonFields) error {
		return ae.Handlers.TransitRouter.Patch(MapPatchTransitRouterToModel(params.ID, params.Body), fields.ConcatNestedNames().FilterMaps("tags"), false)
	})
}
