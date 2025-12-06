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
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/permissions"
	"github.com/openziti/ziti/controller/response"
	"github.com/openziti/ziti/controller/rest_server/operations/service"
)

func init() {
	r := NewFabricServiceRouter()
	env.AddRouter(r)
}

type FabricServiceRouter struct {
	BasePath string
}

func NewFabricServiceRouter() *FabricServiceRouter {
	return &FabricServiceRouter{
		BasePath: "/" + EntityNameService,
	}
}

func (r *FabricServiceRouter) Register(ae *env.AppEnv) {
	ae.FabricApi.ServiceDeleteServiceHandler = service.DeleteServiceHandlerFunc(func(params service.DeleteServiceParams) middleware.Responder {
		return ae.IsAllowed(r.Delete, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.FabricApi.ServiceDetailServiceHandler = service.DetailServiceHandlerFunc(func(params service.DetailServiceParams) middleware.Responder {
		return ae.IsAllowed(r.Detail, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.FabricApi.ServiceListServicesHandler = service.ListServicesHandlerFunc(func(params service.ListServicesParams) middleware.Responder {
		return ae.IsAllowed(r.ListServices, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.FabricApi.ServiceUpdateServiceHandler = service.UpdateServiceHandlerFunc(func(params service.UpdateServiceParams) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Update(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.FabricApi.ServiceCreateServiceHandler = service.CreateServiceHandlerFunc(func(params service.CreateServiceParams) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Create(ae, rc, params) }, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.FabricApi.ServicePatchServiceHandler = service.PatchServiceHandlerFunc(func(params service.PatchServiceParams) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			Patch(rc, func(id string, fields fields.UpdatedFields) error {
				return ae.Managers.Service.Update(
					MapPatchFabricServiceToModel(params.ID, params.Service),
					fields.FilterMaps("tags").MapField("maxIdleTimeMillis", "maxIdleTime"),
					rc.NewChangeContext())
			})
		}, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.FabricApi.ServiceListServiceTerminatorsHandler = service.ListServiceTerminatorsHandlerFunc(func(params service.ListServiceTerminatorsParams) middleware.Responder {
		return ae.IsAllowed(r.listManagementTerminators, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})
}

func (r *FabricServiceRouter) ListServices(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler[*model.Service](ae, rc, ae.Managers.Service, FabricServiceModelMapper{}.ToApi)
}

func (r *FabricServiceRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler[*model.Service](ae, rc, ae.Managers.Service, FabricServiceModelMapper{}.ToApi)
}

func (r *FabricServiceRouter) Create(ae *env.AppEnv, rc *response.RequestContext, params service.CreateServiceParams) {
	CreateFabric(rc, FabricServiceLinkFactory, func() (string, error) {
		svc := MapCreateFabricServiceToModel(params.Service)
		err := ae.Managers.Service.Create(svc, rc.NewChangeContext())
		if err != nil {
			return "", err
		}
		return svc.Id, nil
	})
}

func (r *FabricServiceRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ae.Managers.Service)
}

func (r *FabricServiceRouter) Update(ae *env.AppEnv, rc *response.RequestContext, params service.UpdateServiceParams) {
	Update(rc, func(id string) error {
		return ae.Managers.Service.Update(MapUpdateFabricServiceToModel(params.ID, params.Service), nil, rc.NewChangeContext())
	})
}

func (r *FabricServiceRouter) Patch(ae *env.AppEnv, rc *response.RequestContext, params service.PatchServiceParams) {
	Patch(rc, func(id string, fields fields.UpdatedFields) error {
		return ae.Managers.Service.Update(MapPatchFabricServiceToModel(params.ID, params.Service), fields.FilterMaps("tags"), rc.NewChangeContext())
	})
}

func (r *FabricServiceRouter) listManagementTerminators(ae *env.AppEnv, rc *response.RequestContext) {
	ListAssociationWithHandler[*model.Service, *model.Terminator](ae, rc, ae.Managers.Service, ae.Managers.Terminator, MapFabricTerminatorToRestEntity)
}
