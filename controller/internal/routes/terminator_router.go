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
	r := NewTerminatorRouter()
	env.AddRouter(r)
}

type TerminatorRouter struct {
	BasePath string
	IdType   response.IdType
}

func NewTerminatorRouter() *TerminatorRouter {
	return &TerminatorRouter{
		BasePath: "/" + EntityNameTerminator,
		IdType:   response.IdTypeString,
	}
}

func (ir *TerminatorRouter) Register(ae *env.AppEnv) {
	registerCrudRouter(ae, ae.RootRouter, ir.BasePath, ir, permissions.IsAdmin())
}

func (ir *TerminatorRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler(ae, rc, ae.Handlers.Terminator, MapTerminatorToApiEntity)
}

func (ir *TerminatorRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Handlers.Terminator, MapTerminatorToApiEntity, ir.IdType)
}

func (ir *TerminatorRouter) Create(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &TerminatorApi{}
	Create(rc, rc.RequestResponder, ae.Schemes.Terminator.Post, apiEntity, (&TerminatorApiList{}).BuildSelfLink, func() (string, error) {
		return ae.Handlers.Terminator.Create(apiEntity.ToModel(""))
	})
}

func (ir *TerminatorRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ir.IdType, ae.Handlers.Terminator)
}

func (ir *TerminatorRouter) Update(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &TerminatorApi{}
	Update(rc, ae.Schemes.Terminator.Put, ir.IdType, apiEntity, func(id string) error {
		return ae.Handlers.Terminator.Update(apiEntity.ToModel(id))
	})
}

func (ir *TerminatorRouter) Patch(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &TerminatorApi{}
	Patch(rc, ae.Schemes.Terminator.Patch, ir.IdType, apiEntity, func(id string, fields JsonFields) error {
		return ae.Handlers.Terminator.Patch(apiEntity.ToModel(id), fields.FilterMaps("tags"))
	})
}
