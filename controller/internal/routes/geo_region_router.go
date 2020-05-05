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
	"github.com/openziti/edge/rest_server/operations/geo_region"
)

func init() {
	r := NewGeoRegionRouter()
	env.AddRouter(r)
}

type GeoRegionRouter struct {
	BasePath string
	IdType   response.IdType
}

func NewGeoRegionRouter() *GeoRegionRouter {
	return &GeoRegionRouter{
		BasePath: "/" + EntityNameGeoRegion,
		IdType:   response.IdTypeString,
	}
}

func (r *GeoRegionRouter) Register(ae *env.AppEnv) {

	ae.Api.GeoRegionDetailGeoRegionHandler = geo_region.DetailGeoRegionHandlerFunc(func(params geo_region.DetailGeoRegionParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Detail, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.Api.GeoRegionListGeoRegionsHandler = geo_region.ListGeoRegionsHandlerFunc(func(params geo_region.ListGeoRegionsParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.List, params.HTTPRequest, "", "", permissions.IsAdmin())
	})
}

func (r *GeoRegionRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler(ae, rc, ae.Handlers.GeoRegion, MapGeoRegionToRestEntity)
}

func (r *GeoRegionRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Handlers.GeoRegion, MapGeoRegionToRestEntity)
}
