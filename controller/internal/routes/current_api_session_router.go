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
	"github.com/netfoundry/ziti-edge/controller/env"
	"github.com/netfoundry/ziti-edge/controller/internal/permissions"
	"github.com/netfoundry/ziti-edge/controller/model"
	"github.com/netfoundry/ziti-edge/controller/response"
	"time"

	"fmt"
	"github.com/michaelquigley/pfxlog"
	"net/http"
)

func init() {
	r := NewCurrentSessionRouter()
	env.AddRouter(r)
}

type CurrentSessionRouter struct {
}

func NewCurrentSessionRouter() *CurrentSessionRouter {
	return &CurrentSessionRouter{}
}

func (ir *CurrentSessionRouter) Register(ae *env.AppEnv) {
	detailHandler := ae.WrapHandler(ir.Detail, permissions.IsAuthenticated())
	deleteHandler := ae.WrapHandler(ir.Delete, permissions.IsAuthenticated())

	prefixWithOutSlash := "/" + EntityNameCurrentSession
	prefixWithSlash := prefixWithOutSlash + "/"

	ae.RootRouter.HandleFunc(prefixWithOutSlash, detailHandler).Methods(http.MethodGet)
	ae.RootRouter.HandleFunc(prefixWithSlash, detailHandler).Methods(http.MethodGet)

	ae.RootRouter.HandleFunc(prefixWithOutSlash, deleteHandler).Methods(http.MethodDelete)
	ae.RootRouter.HandleFunc(prefixWithSlash, deleteHandler).Methods(http.MethodDelete)
}

func (ir *CurrentSessionRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	apiSession, err := ir.ToApiDetailEntity(ae, rc, rc.Session)
	if err != nil {
		rc.RequestResponder.RespondWithError(err)
		return
	}

	apiSession.PopulateLinks()
	rc.RequestResponder.RespondWithOk(apiSession, nil)
}

func (ir *CurrentSessionRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	err := ae.GetHandlers().ApiSession.HandleDelete(rc.Session.Id)

	if err != nil {
		rc.RequestResponder.RespondWithError(err)
		return
	}

	rc.RequestResponder.RespondWithOk(nil, nil)
}

func (ir *CurrentSessionRouter) ToApiDetailEntity(ae *env.AppEnv, rc *response.RequestContext, e model.BaseModelEntity) (BaseApiEntity, error) {
	s, ok := e.(*model.ApiSession)

	if !ok {
		err := fmt.Errorf("entity is not an api session \"%s\"", e.GetId())
		log := pfxlog.Logger()
		log.WithField("id", e.GetId()).
			Error(err)
		return nil, err
	}

	return RenderCurrentSessionApiListEntity(s, ae.Config.SessionTimeoutDuration())
}

func RenderCurrentSessionApiListEntity(s *model.ApiSession, sessionTimeout time.Duration) (BaseApiEntity, error) {
	baseApi := env.FromBaseModelEntity(s)

	expiresAt := s.UpdatedAt.Add(sessionTimeout)

	ret := &CurrentSessionApiList{
		BaseApi:   baseApi,
		Token:     &s.Token,
		ExpiresAt: &expiresAt,
		Identity:  NewIdentityEntityRef(s.Identity),
	}

	ret.PopulateLinks()

	return ret, nil
}

func (ir *CurrentSessionRouter) GetIdFromRequest(r *http.Request, rctx *response.RequestContext) (string, error) {
	return rctx.Session.Id, nil
}

func (ir *CurrentSessionRouter) GetIdPropertyName() string {
	return "requestContextSessionId"
}
