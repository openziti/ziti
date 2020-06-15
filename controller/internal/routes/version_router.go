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
	"github.com/openziti/edge/build"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/internal/permissions"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/edge/rest_server/operations/informational"
	"runtime"
)

func init() {
	r := NewVersionRouter()
	env.AddRouter(r)
}

type VersionRouter struct {
	BasePath string
}

func NewVersionRouter() *VersionRouter {
	return &VersionRouter{
		BasePath: "/version",
	}
}

func (ir *VersionRouter) Register(ae *env.AppEnv) {
	ae.Api.InformationalListVersionHandler = informational.ListVersionHandlerFunc(func(params informational.ListVersionParams) middleware.Responder {
		return ae.IsAllowed(ir.List, params.HTTPRequest, "", "", permissions.Always())
	})

	ae.Api.InformationalListRootHandler = informational.ListRootHandlerFunc(func(params informational.ListRootParams) middleware.Responder {
		return ae.IsAllowed(ir.List, params.HTTPRequest, "", "", permissions.Always())
	})
}

func (ir *VersionRouter) List(ae *env.AppEnv, rc *response.RequestContext) {

	buildInfo := build.GetBuildInfo()
	data := rest_model.Version{
		BuildDate:      buildInfo.GetBuildDate(),
		Revision:       buildInfo.GetRevision(),
		RuntimeVersion: runtime.Version(),
		Version:        buildInfo.GetVersion(),
	}

	rc.RespondWithOk(data, nil)
}
