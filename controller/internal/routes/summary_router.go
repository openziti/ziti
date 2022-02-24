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
	"github.com/openziti/edge/rest_management_api_server/operations/informational"
	"github.com/openziti/edge/rest_model"
)

func init() {
	r := NewSummaryRouter()
	env.AddRouter(r)
}

type SummaryRouter struct {
	BasePath string
}

func NewSummaryRouter() *SummaryRouter {
	return &SummaryRouter{
		BasePath: "/summary",
	}
}

func (r *SummaryRouter) Register(ae *env.AppEnv) {
	ae.ManagementApi.InformationalListSummaryHandler = informational.ListSummaryHandlerFunc(func(params informational.ListSummaryParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.List, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

}

func (r *SummaryRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	data, err := ae.GetStores().GetEntityCounts(ae.GetDbProvider())
	if err != nil {
		rc.RespondWithError(err)
	} else {
		rc.RespondWithOk(rest_model.ListSummaryCounts(data), nil)
	}
}
