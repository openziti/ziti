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
	"github.com/openziti/edge/rest_server/operations/posture_checks"
)

func init() {
	r := NewPostureCheckRouter()
	env.AddRouter(r)
}

type PostureCheckRouter struct {
	BasePath string
}

func NewPostureCheckRouter() *PostureCheckRouter {
	return &PostureCheckRouter{
		BasePath: "/" + EntityNamePostureCheck,
	}
}

func (r *PostureCheckRouter) Register(ae *env.AppEnv) {
	ae.Api.PostureChecksDeletePostureCheckHandler = posture_checks.DeletePostureCheckHandlerFunc(func(params posture_checks.DeletePostureCheckParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Delete, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.Api.PostureChecksDetailPostureCheckHandler = posture_checks.DetailPostureCheckHandlerFunc(func(params posture_checks.DetailPostureCheckParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Detail, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.Api.PostureChecksListPostureChecksHandler = posture_checks.ListPostureChecksHandlerFunc(func(params posture_checks.ListPostureChecksParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.List, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.Api.PostureChecksUpdatePostureCheckHandler = posture_checks.UpdatePostureCheckHandlerFunc(func(params posture_checks.UpdatePostureCheckParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Update(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.Api.PostureChecksCreatePostureCheckHandler = posture_checks.CreatePostureCheckHandlerFunc(func(params posture_checks.CreatePostureCheckParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Create(ae, rc, params) }, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.Api.PostureChecksPatchPostureCheckHandler = posture_checks.PatchPostureCheckHandlerFunc(func(params posture_checks.PatchPostureCheckParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Patch(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})
}

func (r *PostureCheckRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler(ae, rc, ae.Handlers.PostureCheck, MapPostureCheckToRestEntity)
}

func (r *PostureCheckRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Handlers.PostureCheck, MapPostureCheckToRestEntity)
}

func (r *PostureCheckRouter) Create(ae *env.AppEnv, rc *response.RequestContext, params posture_checks.CreatePostureCheckParams) {
	Create(rc, rc, PostureCheckLinkFactory, func() (string, error) {
		return ae.Handlers.PostureCheck.Create(MapCreatePostureCheckToModel(params.Body))
	})
}

func (r *PostureCheckRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ae.Handlers.PostureCheck)
}

func (r *PostureCheckRouter) Update(ae *env.AppEnv, rc *response.RequestContext, params posture_checks.UpdatePostureCheckParams) {
	Update(rc, func(id string) error {
		return ae.Handlers.PostureCheck.Update(MapUpdatePostureCheckToModel(params.ID, params.Body))
	})
}

func (r *PostureCheckRouter) Patch(ae *env.AppEnv, rc *response.RequestContext, params posture_checks.PatchPostureCheckParams) {
	Patch(rc, func(id string, fields JsonFields) error {
		return ae.Handlers.PostureCheck.Patch(MapPatchPostureCheckToModel(params.ID, params.Body), fields.FilterMaps("tags"))
	})
}
