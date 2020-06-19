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
	"errors"
	"github.com/go-openapi/runtime/middleware"
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/internal/permissions"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_server/operations/database"
	"github.com/openziti/fabric/controller/network"
)

func init() {
	r := NewDatabaseRouter()
	env.AddRouter(r)
}

type DatabaseRouter struct {
}

func NewDatabaseRouter() *DatabaseRouter {
	return &DatabaseRouter{}
}

func (r *DatabaseRouter) Register(ae *env.AppEnv) {
	ae.Api.DatabaseCreateDatabaseSnapshotHandler = database.CreateDatabaseSnapshotHandlerFunc(func(params database.CreateDatabaseSnapshotParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.CreateSnapshot(ae, rc) }, params.HTTPRequest, "", "", permissions.IsAdmin())
	})
}

func (r *DatabaseRouter) CreateSnapshot(ae *env.AppEnv, rc *response.RequestContext) {
	if err := ae.HostController.GetNetwork().SnapshotDatabase(); err != nil {
		if errors.Is(err, network.DbSnapshotTooFrequentError) {
			rc.RespondWithApiError(apierror.NewRateLimited())
			return
		}
		rc.RespondWithError(err)
		return
	}
	rc.RespondWithEmptyOk()
}
