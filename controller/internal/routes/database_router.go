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
	"github.com/go-openapi/strfmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/internal/permissions"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_management_api_server/operations/database"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/foundation/util/concurrenz"
	"net/http"
	"sync"
	"time"
)

func init() {
	r := NewDatabaseRouter()
	env.AddRouter(r)
}

type integrityCheckOp struct {
	running       concurrenz.AtomicBoolean
	results       []*rest_model.DataIntegrityCheckDetail
	lock          sync.Mutex
	fixingErrors  bool
	startTime     *time.Time
	endTime       *time.Time
	err           error
	tooManyErrors bool
}

type DatabaseRouter struct {
	integrityCheck integrityCheckOp
}

func NewDatabaseRouter() *DatabaseRouter {
	return &DatabaseRouter{}
}

func (r *DatabaseRouter) Register(ae *env.AppEnv) {
	ae.ManagementApi.DatabaseCreateDatabaseSnapshotHandler = database.CreateDatabaseSnapshotHandlerFunc(func(params database.CreateDatabaseSnapshotParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.CreateSnapshot(ae, rc) }, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.DatabaseCheckDataIntegrityHandler = database.CheckDataIntegrityHandlerFunc(func(params database.CheckDataIntegrityParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.CheckDatastoreIntegrity(ae, rc, false) }, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.DatabaseFixDataIntegrityHandler = database.FixDataIntegrityHandlerFunc(func(params database.FixDataIntegrityParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.CheckDatastoreIntegrity(ae, rc, true) }, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.DatabaseDataIntegrityResultsHandler = database.DataIntegrityResultsHandlerFunc(func(params database.DataIntegrityResultsParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.GetCheckProgress(rc) }, params.HTTPRequest, "", "", permissions.IsAdmin())
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

func (r *DatabaseRouter) CheckDatastoreIntegrity(ae *env.AppEnv, rc *response.RequestContext, fixErrors bool) {
	if r.integrityCheck.running.CompareAndSwap(false, true) {
		r.integrityCheck.fixingErrors = fixErrors
		go r.runDataIntegrityCheck(ae, fixErrors)
		rc.Respond(&rest_model.Empty{Data: map[string]interface{}{}, Meta: &rest_model.Meta{}}, http.StatusAccepted)
	} else {
		rc.RespondWithApiError(apierror.NewRateLimited())
	}
}

func (r *DatabaseRouter) GetCheckProgress(rc *response.RequestContext) {
	integrityCheck := r.integrityCheck

	integrityCheck.lock.Lock()
	defer integrityCheck.lock.Unlock()

	limit := int64(-1)
	zero := int64(0)
	count := int64(len(integrityCheck.results))

	var err *string
	if integrityCheck.err != nil {
		errStr := integrityCheck.err.Error()
		err = &errStr
	}

	inProgress := integrityCheck.running.Get()

	result := rest_model.DataIntegrityCheckResultEnvelope{
		Data: &rest_model.DataIntegrityCheckDetails{
			EndTime:       (*strfmt.DateTime)(integrityCheck.endTime),
			Error:         err,
			FixingErrors:  &integrityCheck.fixingErrors,
			InProgress:    &inProgress,
			Results:       integrityCheck.results,
			StartTime:     (*strfmt.DateTime)(integrityCheck.startTime),
			TooManyErrors: &integrityCheck.tooManyErrors,
		},
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

func (r *DatabaseRouter) runDataIntegrityCheck(ae *env.AppEnv, fixErrors bool) {
	defer func() {
		r.integrityCheck.lock.Lock()
		now := time.Now()
		r.integrityCheck.endTime = &now
		r.integrityCheck.running.Set(false)
		r.integrityCheck.lock.Unlock()
	}()

	r.integrityCheck.lock.Lock()
	now := time.Now()
	r.integrityCheck.results = nil
	r.integrityCheck.startTime = &now
	r.integrityCheck.endTime = nil
	r.integrityCheck.err = nil
	r.integrityCheck.tooManyErrors = false
	r.integrityCheck.lock.Unlock()

	logger := pfxlog.Logger()

	errorHandler := func(err error, fixed bool) {
		logger.WithError(err).Warnf("data integrity error reported. fixed? %v", fixed)

		r.integrityCheck.lock.Lock()
		defer r.integrityCheck.lock.Unlock()

		if len(r.integrityCheck.results) < 1000 {
			description := err.Error()
			r.integrityCheck.results = append(r.integrityCheck.results, &rest_model.DataIntegrityCheckDetail{
				Description: &description,
				Fixed:       &fixed,
			})
		} else {
			r.integrityCheck.tooManyErrors = true
		}
	}

	r.integrityCheck.err = ae.GetDbProvider().GetStores().CheckIntegrity(ae.GetDbProvider().GetDb(), fixErrors, errorHandler)
}
