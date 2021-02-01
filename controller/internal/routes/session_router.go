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
	"github.com/openziti/edge/rest_server/operations/session"
	"github.com/openziti/foundation/metrics"
	"time"
)

func init() {
	r := NewSessionRouter()
	env.AddRouter(r)
}

type SessionRouter struct {
	BasePath    string
	IdType      response.IdType
	createTimer metrics.Timer
}

func NewSessionRouter() *SessionRouter {
	return &SessionRouter{
		BasePath: "/" + EntityNameSession,
	}
}

func (r *SessionRouter) Register(ae *env.AppEnv) {
	r.createTimer = ae.GetHostController().GetNetwork().GetMetricsRegistry().Timer("session.create")
	ae.Api.SessionDeleteSessionHandler = session.DeleteSessionHandlerFunc(func(params session.DeleteSessionParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Delete, params.HTTPRequest, params.ID, "", permissions.IsAuthenticated())
	})

	ae.Api.SessionDetailSessionHandler = session.DetailSessionHandlerFunc(func(params session.DetailSessionParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Detail, params.HTTPRequest, params.ID, "", permissions.IsAuthenticated())
	})

	ae.Api.SessionListSessionsHandler = session.ListSessionsHandlerFunc(func(params session.ListSessionsParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.List, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})

	ae.Api.SessionCreateSessionHandler = session.CreateSessionHandlerFunc(func(params session.CreateSessionParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Create(ae, rc, params) }, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})
}

func (r *SessionRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	// ListWithHandler won't do search limiting by logged in user
	List(rc, func(rc *response.RequestContext, queryOptions *PublicQueryOptions) (*QueryResult, error) {
		query, err := queryOptions.getFullQuery(ae.Handlers.Session.GetStore())
		if err != nil {
			return nil, err
		}

		result, err := ae.Handlers.Session.PublicQueryForIdentity(rc.Identity, query)
		if err != nil {
			return nil, err
		}
		sessions, err := MapSessionsToRestEntities(ae, rc, result.Sessions)
		if err != nil {
			return nil, err
		}
		return NewQueryResult(sessions, &result.QueryMetaData), nil
	})
}

func (r *SessionRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	// DetailWithHandler won't do search limiting by logged in user
	Detail(rc, func(rc *response.RequestContext, id string) (interface{}, error) {
		service, err := ae.Handlers.Session.ReadForIdentity(id, rc.ApiSession.IdentityId)
		if err != nil {
			return nil, err
		}
		return MapSessionToRestEntity(ae, rc, service)
	})
}

func (r *SessionRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	Delete(rc, func(rc *response.RequestContext, id string) error {
		return ae.Handlers.Session.DeleteForIdentity(id, rc.ApiSession.IdentityId)
	})
}

func (r *SessionRouter) Create(ae *env.AppEnv, rc *response.RequestContext, params session.CreateSessionParams) {
	start := time.Now()
	responder := &SessionRequestResponder{ae: ae, Responder: rc}
	CreateWithResponder(rc, responder, SessionLinkFactory, func() (string, error) {
		return ae.Handlers.Session.Create(MapCreateSessionToModel(rc.ApiSession.Id, params.Body))
	})
	r.createTimer.UpdateSince(start)
}
