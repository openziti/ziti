/*
	Copyright 2019 Netfoundry, Inc.

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
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/env"
	"github.com/netfoundry/ziti-edge/controller/internal/permissions"
	"github.com/netfoundry/ziti-edge/controller/model"
	"github.com/netfoundry/ziti-edge/controller/response"
)

func init() {
	r := NewSessionRouter()
	env.AddRouter(r)
}

type SessionRouter struct {
	BasePath       string
	LegacyBasePath string
	IdType         response.IdType
}

func NewSessionRouter() *SessionRouter {
	return &SessionRouter{
		BasePath:       "/" + EntityNameSession,
		LegacyBasePath: "/" + EntityNameSessionLegacy,
		IdType:         response.IdTypeUuid,
	}
}

func (ir *SessionRouter) Register(ae *env.AppEnv) {
	registerCreateReadDeleteRouter(ae, ae.RootRouter, ir.BasePath, ir, &crudResolvers{
		Create:  permissions.IsAuthenticated(),
		Read:    permissions.IsAuthenticated(),
		Delete:  permissions.IsAuthenticated(),
		Default: permissions.IsAdmin(),
	})

	registerCreateReadDeleteRouter(ae, ae.RootRouter, ir.LegacyBasePath, ir, &crudResolvers{
		Create:  permissions.IsAuthenticated(),
		Read:    permissions.IsAuthenticated(),
		Delete:  permissions.IsAuthenticated(),
		Default: permissions.IsAdmin(),
	})
}

func (ir *SessionRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	// ListWithHandler won't do search limiting by logged in user
	List(rc, func(rc *response.RequestContext, queryOptions *model.QueryOptions) (*QueryResult, error) {
		result, err := ae.Handlers.Session.PublicQueryForIdentity(rc.Identity, queryOptions)
		if err != nil {
			return nil, err
		}
		sessions, err := MapSessionsToApiEntities(ae, rc, result.Sessions)
		if err != nil {
			return nil, err
		}
		return NewQueryResult(sessions, &result.QueryMetaData), nil
	})
}

func (ir *SessionRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	// DetailWithHandler won't do search limiting by logged in user
	Detail(rc, ir.IdType, func(rc *response.RequestContext, id string) (BaseApiEntity, error) {
		service, err := ae.Handlers.Session.ReadForIdentity(id, rc.ApiSession.IdentityId)
		if err != nil {
			return nil, err
		}
		return MapSessionToApiEntity(ae, rc, service)
	})
}

func (ir *SessionRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	Delete(rc, ir.IdType, func(rc *response.RequestContext, id string) error {
		return ae.Handlers.Session.DeleteForIdentity(id, rc.ApiSession.IdentityId)
	})
}

func (ir *SessionRouter) Create(ae *env.AppEnv, rc *response.RequestContext) {
	//todo re-enable this check w/ a new auth table or allow any auth'ed session to have a short term NS cert
	//if rc.Identity.AuthenticatorCert == nil {
	//	rc.RequestResponder.RespondWithApiError(&response.ApiError{
	//		Code:           response.NetworkSessionsRequireCertificateAuthCode,
	//		Message:        response.NetworkSessionsRequireCertificateAuthMessage,
	//		HttpStatusCode: http.StatusBadRequest,
	//	})
	//	return
	//}

	sessionCreate := &SessionApiPost{}
	responder := &SessionRequestResponder{ae: ae, RequestResponder: rc.RequestResponder}
	Create(rc, responder, ae.Schemes.Session.Post, sessionCreate, (&SessionApiList{}).BuildSelfLink, func() (string, error) {
		return ae.Handlers.Session.Create(sessionCreate.ToModel(rc))
	})
}

type SessionRequestResponder struct {
	response.RequestResponder
	ae *env.AppEnv
}

type SessionEdgeRouter struct {
	Hostname *string           `json:"hostname"`
	Name     *string           `json:"name"`
	Urls     map[string]string `json:"urls"`
}

func getSessionEdgeRouters(ae *env.AppEnv, ns *model.Session) ([]*SessionEdgeRouter, error) {
	var edgeRouters []*SessionEdgeRouter

	edgeRoutersForSession, err := ae.Handlers.EdgeRouter.ListForSession(ns.Id)
	if err != nil {
		return nil, err
	}

	for _, edgeRouter := range edgeRoutersForSession.EdgeRouters {
		onlineEdgeRouter := ae.Broker.GetOnlineEdgeRouter(edgeRouter.Id)

		if onlineEdgeRouter != nil {
			c := &SessionEdgeRouter{
				Hostname: onlineEdgeRouter.Hostname,
				Name:     &edgeRouter.Name,
				Urls:     map[string]string{},
			}

			for p, url := range onlineEdgeRouter.EdgeRouterProtocols {
				c.Urls[p] = url
			}

			pfxlog.Logger().Infof("Returning %+v to %+v, with urls: %+v", edgeRouter, c, c.Urls)
			edgeRouters = append(edgeRouters, c)
		}
	}

	return edgeRouters, nil
}

func (nsr *SessionRequestResponder) RespondWithCreatedId(id string, link *response.Link) {
	modelSession, err := nsr.ae.GetHandlers().Session.Read(id)
	if err != nil {
		nsr.RespondWithError(err)
		return
	}

	apiSession, err := MapSessionToApiList(nsr.ae, modelSession)
	if err != nil {
		nsr.RespondWithError(err)
		return
	}
	newSession := &NewSession{
		SessionApiList: apiSession,
		Token:          modelSession.Token,
	}
	nsr.RespondWithCreated(newSession, nil, link)
}
