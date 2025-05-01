/*
	Copyright NetFoundry Inc.

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
	"github.com/openziti/edge-api/rest_management_api_server/operations/authenticator"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/fields"
	"github.com/openziti/ziti/controller/internal/permissions"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/response"
	"github.com/pkg/errors"
	"time"
)

func init() {
	r := NewAuthenticatorRouter()
	env.AddRouter(r)
}

type AuthenticatorRouter struct {
	BasePath string
}

func NewAuthenticatorRouter() *AuthenticatorRouter {
	return &AuthenticatorRouter{
		BasePath: "/" + EntityNameAuthenticator,
	}
}

func (r *AuthenticatorRouter) Register(ae *env.AppEnv) {
	ae.ManagementApi.AuthenticatorDeleteAuthenticatorHandler = authenticator.DeleteAuthenticatorHandlerFunc(func(params authenticator.DeleteAuthenticatorParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Delete, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.AuthenticatorDetailAuthenticatorHandler = authenticator.DetailAuthenticatorHandlerFunc(func(params authenticator.DetailAuthenticatorParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Detail, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.AuthenticatorListAuthenticatorsHandler = authenticator.ListAuthenticatorsHandlerFunc(func(params authenticator.ListAuthenticatorsParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.List, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.AuthenticatorUpdateAuthenticatorHandler = authenticator.UpdateAuthenticatorHandlerFunc(func(params authenticator.UpdateAuthenticatorParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Update(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.AuthenticatorCreateAuthenticatorHandler = authenticator.CreateAuthenticatorHandlerFunc(func(params authenticator.CreateAuthenticatorParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Create(ae, rc, params) }, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.AuthenticatorPatchAuthenticatorHandler = authenticator.PatchAuthenticatorHandlerFunc(func(params authenticator.PatchAuthenticatorParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Patch(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.AuthenticatorReEnrollAuthenticatorHandler = authenticator.ReEnrollAuthenticatorHandlerFunc(func(params authenticator.ReEnrollAuthenticatorParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.ReEnroll(ae, rc, params)
		}, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.AuthenticatorRequestExtendAuthenticatorHandler = authenticator.RequestExtendAuthenticatorHandlerFunc(func(params authenticator.RequestExtendAuthenticatorParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.RequestExtend(ae, rc, params)
		}, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})
}

func (r *AuthenticatorRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler[*model.Authenticator](ae, rc, ae.Managers.Authenticator, MapAuthenticatorToRestEntity)
}

func (r *AuthenticatorRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler[*model.Authenticator](ae, rc, ae.Managers.Authenticator, MapAuthenticatorToRestEntity)
}

func (r *AuthenticatorRouter) Create(ae *env.AppEnv, rc *response.RequestContext, params authenticator.CreateAuthenticatorParams) {
	Create(rc, rc, AuthenticatorLinkFactory, func() (string, error) {
		authenticator, err := MapCreateToAuthenticatorModel(params.Authenticator)

		if err != nil {
			return "", err
		}

		return MapCreate(ae.Managers.Authenticator.Create, authenticator, rc)
	})
}

func (r *AuthenticatorRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ae.Managers.Authenticator)
}

func (r *AuthenticatorRouter) Update(ae *env.AppEnv, rc *response.RequestContext, params authenticator.UpdateAuthenticatorParams) {
	Update(rc, func(id string) error {
		return ae.Managers.Authenticator.Update(MapUpdateAuthenticatorToModel(params.ID, params.Authenticator), false, nil, rc.NewChangeContext())
	})
}

func (r *AuthenticatorRouter) Patch(ae *env.AppEnv, rc *response.RequestContext, params authenticator.PatchAuthenticatorParams) {
	Patch(rc, func(id string, fields fields.UpdatedFields) error {
		model := MapPatchAuthenticatorToModel(params.ID, params.Authenticator)

		if fields.IsUpdated("password") {
			fields.AddField("salt")
		}

		return ae.Managers.Authenticator.Update(model, false, fields.FilterMaps("tags"), rc.NewChangeContext())
	})
}

func (r *AuthenticatorRouter) ReEnroll(ae *env.AppEnv, rc *response.RequestContext, params authenticator.ReEnrollAuthenticatorParams) {
	id, err := rc.GetEntityId()

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	if id == "" {
		rc.RespondWithError(errors.New("entity id missing"))
		return
	}

	if id, err := ae.Managers.Authenticator.ReEnroll(id, time.Time(*params.ReEnroll.ExpiresAt), rc.NewChangeContext()); err == nil {
		rc.RespondWithCreatedId(id, EnrollmentLinkFactory.SelfLinkFromId(id))
	} else {
		if fe, ok := err.(*errorz.FieldError); ok {
			rc.RespondWithFieldError(fe)
			return
		}
		rc.RespondWithError(err)
		return
	}
}

func (r *AuthenticatorRouter) RequestExtend(ae *env.AppEnv, rc *response.RequestContext, params authenticator.RequestExtendAuthenticatorParams) {
	id, err := rc.GetEntityId()

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	if id == "" {
		rc.RespondWithError(errors.New("entity id missing"))
		return
	}

	err = ae.Managers.Authenticator.RequestExtend(id, params.RequestExtendAuthenticator.RollKeys, rc.NewChangeContext())

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	rc.RespondWithEmptyOk()
}
