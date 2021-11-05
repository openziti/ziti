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

package api_impl

import (
	"github.com/go-openapi/runtime/middleware"
	"github.com/openziti/fabric/controller/api"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/rest_server/operations"
	"github.com/openziti/fabric/rest_server/operations/router"
)

func init() {
	r := NewRouterRouter()
	AddRouter(r)
}

type RouterRouter struct {
	BasePath string
}

func NewRouterRouter() *RouterRouter {
	return &RouterRouter{
		BasePath: "/" + EntityNameRouter,
	}
}

func (r *RouterRouter) Register(fabricApi *operations.ZitiFabricAPI, wrapper RequestWrapper) {
	fabricApi.RouterDeleteRouterHandler = router.DeleteRouterHandlerFunc(func(params router.DeleteRouterParams) middleware.Responder {
		return wrapper.WrapRequest(r.Delete, params.HTTPRequest, params.ID, "")
	})

	fabricApi.RouterDetailRouterHandler = router.DetailRouterHandlerFunc(func(params router.DetailRouterParams) middleware.Responder {
		return wrapper.WrapRequest(r.Detail, params.HTTPRequest, params.ID, "")
	})

	fabricApi.RouterListRoutersHandler = router.ListRoutersHandlerFunc(func(params router.ListRoutersParams) middleware.Responder {
		return wrapper.WrapRequest(r.ListRouters, params.HTTPRequest, "", "")
	})

	fabricApi.RouterCreateRouterHandler = router.CreateRouterHandlerFunc(func(params router.CreateRouterParams) middleware.Responder {
		return wrapper.WrapRequest(func(n *network.Network, rc api.RequestContext) { r.Create(n, rc, params) }, params.HTTPRequest, "", "")
	})

	//fabricApi.RouterUpdateRouterHandler = router.UpdateRouterHandlerFunc(func(params router.UpdateRouterParams) middleware.Responder {
	//	return wrapper.WrapRequest(func(n *network.Network, rc api.RequestContext) { r.Update(n, rc, params) }, params.HTTPRequest, params.ID, "")
	//})
	//
	//fabricApi.RouterPatchRouterHandler = router.PatchRouterHandlerFunc(func(params router.PatchRouterParams) middleware.Responder {
	//	return wrapper.WrapRequest(func(n *network.Network, rc api.RequestContext) { r.Patch(n, rc, params) }, params.HTTPRequest, params.ID, "")
	//})

	fabricApi.RouterListRouterTerminatorsHandler = router.ListRouterTerminatorsHandlerFunc(func(params router.ListRouterTerminatorsParams) middleware.Responder {
		return wrapper.WrapRequest(r.listManagementTerminators, params.HTTPRequest, params.ID, "")
	})
}

func (r *RouterRouter) ListRouters(n *network.Network, rc api.RequestContext) {
	ListWithHandler(n, rc, n.Controllers.Routers, MapRouterToRestEntity)
}

func (r *RouterRouter) Detail(n *network.Network, rc api.RequestContext) {
	DetailWithHandler(n, rc, n.Controllers.Routers, MapRouterToRestEntity)
}

func (r *RouterRouter) Create(n *network.Network, rc api.RequestContext, params router.CreateRouterParams) {
	Create(rc, RouterLinkFactory, func() (string, error) {
		router := MapCreateRouterToModel(params.Router)
		err := n.Routers.Create(router)
		if err != nil {
			return "", err
		}
		return router.Id, nil
	})
}

func (r *RouterRouter) Delete(network *network.Network, rc api.RequestContext) {
	DeleteWithHandler(rc, network.Controllers.Routers)
}

//func (r *RouterRouter) Update(n *network.Network, rc api.RequestContext, params router.UpdateRouterParams) {
//	Update(rc, func(id string) error {
//		return n.Controllers.Routers.Update(MapUpdateRouterToModel(params.ID, params.Router))
//	})
//}
//
//func (r *RouterRouter) Patch(n *network.Network, rc api.RequestContext, params router.PatchRouterParams) {
//	Patch(rc, func(id string, fields api.JsonFields) error {
//		return n.Controllers.Routers.Patch(MapPatchRouterToModel(params.ID, params.Router), fields.ConcatNestedNames().FilterMaps("tags"))
//	})
//}

func (r *RouterRouter) listManagementTerminators(n *network.Network, rc api.RequestContext) {
	r.listAssociations(n, rc, n.Controllers.Terminators, MapTerminatorToRestEntity)
}

func (r *RouterRouter) listAssociations(n *network.Network, rc api.RequestContext, associationLoader models.EntityRetriever, mapper ModelToApiMapper) {
	ListAssociationWithHandler(n, rc, n.Controllers.Routers, associationLoader, mapper)
}
