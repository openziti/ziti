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
	"github.com/netfoundry/ziti-edge/controller/env"
	"github.com/netfoundry/ziti-edge/controller/internal/permissions"
	"github.com/netfoundry/ziti-edge/controller/response"
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

func (ir *TransitRouterRouter) Register(ae *env.AppEnv) {
	_ = registerCrudRouter(ae, ae.RootRouter, ir.BasePath, ir, &crudResolvers{
		Create:  permissions.IsAdmin(),
		Read:    permissions.IsAdmin(),
		Update:  permissions.IsAdmin(),
		Delete:  permissions.IsAdmin(),
		Default: permissions.IsAdmin(),
	})
}

func (ir *TransitRouterRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler(ae, rc, ae.Handlers.TransitRouter, MapTransitRouterToApiEntity)
}

func (ir *TransitRouterRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Handlers.TransitRouter, MapTransitRouterToApiEntity, ir.IdType)
}

func (ir *TransitRouterRouter) Create(ae *env.AppEnv, rc *response.RequestContext) {
	transitRouterCreate := &TransitRouterApi{}
	Create(rc, rc.RequestResponder, ae.Schemes.TransitRouter.Post, transitRouterCreate, (&TransitRouterApiList{}).BuildSelfLink, func() (string, error) {
		return ae.Handlers.TransitRouter.Create(transitRouterCreate.ToModel(""))
	})
}

func (ir *TransitRouterRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ir.IdType, ae.Handlers.TransitRouter)
}

func (ir *TransitRouterRouter) Update(ae *env.AppEnv, rc *response.RequestContext) {
	transitRouterUpdate := &TransitRouterApi{}
	Update(rc, ae.Schemes.TransitRouter.Put, ir.IdType, transitRouterUpdate, func(id string) error {
		return ae.Handlers.TransitRouter.Update(transitRouterUpdate.ToModel(id))
	})
}

func (ir *TransitRouterRouter) Patch(ae *env.AppEnv, rc *response.RequestContext) {
	transitRouterUpdate := &TransitRouterApi{}
	Patch(rc, ae.Schemes.TransitRouter.Patch, ir.IdType, transitRouterUpdate, func(id string, fields JsonFields) error {
		return ae.Handlers.TransitRouter.Patch(transitRouterUpdate.ToModel(id), fields.ConcatNestedNames().FilterMaps("tags"))
	})
}
