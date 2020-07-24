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
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/edge/rest_server/operations/database"
	"github.com/openziti/fabric/controller/network"
	"net/http"
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

	ae.Api.DatabaseCheckDataIntegrityHandler = database.CheckDataIntegrityHandlerFunc(func(params database.CheckDataIntegrityParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.CheckDatastoreIntegrity(ae, rc, params.FixErrors) }, params.HTTPRequest, "", "", permissions.IsAdmin())
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

func (r *DatabaseRouter) CheckDatastoreIntegrity(ae *env.AppEnv, rc *response.RequestContext, fixErrors *bool) {
	fix := false
	if fixErrors != nil {
		fix = *fixErrors
	}

	var results []*rest_model.DataIntegrityCheckDetail

	errorHandler := func(err error, fixed bool) {
		description := err.Error()
		results = append(results, &rest_model.DataIntegrityCheckDetail{
			Description: &description,
			Fixed:       &fixed,
		})
	}

	if err := ae.GetStores().CheckIntegrity(fix, errorHandler); err != nil {
		rc.RespondWithError(err)
		return
	}

	limit := int64(-1)
	zero := int64(0)
	count := int64(len(results))

	result := rest_model.DataIntegrityCheckResultEnvelope{
		Data: results,
		Meta: &rest_model.Meta{
			Pagination: &rest_model.Pagination{
				Limit:      &limit,
				Offset:     &zero,
				TotalCount: &count,
			},
			FilterableFields: make([]string, 0),
		},
	}

	rc.Respond(result, http.StatusOK)
}
