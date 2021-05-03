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
	"github.com/openziti/edge/controller"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/internal/permissions"
	"github.com/openziti/edge/controller/response"
	clientInformational "github.com/openziti/edge/rest_client_api_server/operations/informational"
	managementInformational "github.com/openziti/edge/rest_management_api_server/operations/informational"
	"github.com/openziti/edge/rest_model"
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
	ae.ClientApi.InformationalListVersionHandler = clientInformational.ListVersionHandlerFunc(func(params clientInformational.ListVersionParams) middleware.Responder {
		return ae.IsAllowed(ir.List, params.HTTPRequest, "", "", permissions.Always())
	})

	ae.ClientApi.InformationalListRootHandler = clientInformational.ListRootHandlerFunc(func(params clientInformational.ListRootParams) middleware.Responder {
		return ae.IsAllowed(ir.List, params.HTTPRequest, "", "", permissions.Always())
	})

	ae.ManagementApi.InformationalListVersionHandler = managementInformational.ListVersionHandlerFunc(func(params managementInformational.ListVersionParams) middleware.Responder {
		return ae.IsAllowed(ir.List, params.HTTPRequest, "", "", permissions.Always())
	})

	ae.ManagementApi.InformationalListRootHandler = managementInformational.ListRootHandlerFunc(func(params managementInformational.ListRootParams) middleware.Responder {
		return ae.IsAllowed(ir.List, params.HTTPRequest, "", "", permissions.Always())
	})
}

func (ir *VersionRouter) List(_ *env.AppEnv, rc *response.RequestContext) {
	buildInfo := build.GetBuildInfo()
	data := rest_model.Version{
		BuildDate:      buildInfo.BuildDate(),
		Revision:       buildInfo.Revision(),
		RuntimeVersion: runtime.Version(),
		Version:        buildInfo.Version(),
		APIVersions: map[string]map[string]rest_model.APIVersion{
			"edge": {controller.RestApiV1: mapApiVersionToRestModel(controller.LegacyClientRestApiBaseUrlV1)},
		},
	}
	rc.RespondWithOk(data, &rest_model.Meta{})
}

func mapApiVersionToRestModel(path string) rest_model.APIVersion {
	return rest_model.APIVersion{
		Path:    &path,
	}
}