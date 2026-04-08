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
	"net/http"
	"time"

	"github.com/go-openapi/runtime/middleware"
	"github.com/google/uuid"
	"github.com/openziti/edge-api/rest_management_api_server/operations/revocation"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/v2/controller/env"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/models"
	"github.com/openziti/ziti/v2/controller/permissions"
	"github.com/openziti/ziti/v2/controller/response"
)

func init() {
	r := NewRevocationRouter()
	env.AddRouter(r)
}

// RevocationRouter handles Management API routes for revocation management.
type RevocationRouter struct {
	BasePath string
}

// NewRevocationRouter creates a new RevocationRouter.
func NewRevocationRouter() *RevocationRouter {
	return &RevocationRouter{
		BasePath: "/" + EntityNameRevocations,
	}
}

func (r *RevocationRouter) Register(ae *env.AppEnv) {
	ae.ManagementApi.RevocationListRevocationsHandler = revocation.ListRevocationsHandlerFunc(func(params revocation.ListRevocationsParams, _ interface{}) middleware.Responder {
		ae.InitPermissionsContext(params.HTTPRequest, permissions.Management, "revocation", permissions.Read)
		return ae.IsAllowed(r.List, params.HTTPRequest, "", "", permissions.DefaultManagementAccess())
	})

	ae.ManagementApi.RevocationDetailRevocationHandler = revocation.DetailRevocationHandlerFunc(func(params revocation.DetailRevocationParams, _ interface{}) middleware.Responder {
		ae.InitPermissionsContext(params.HTTPRequest, permissions.Management, "revocation", permissions.Read)
		return ae.IsAllowed(r.Detail, params.HTTPRequest, params.ID, "", permissions.DefaultManagementAccess())
	})

	ae.ManagementApi.RevocationCreateRevocationHandler = revocation.CreateRevocationHandlerFunc(func(params revocation.CreateRevocationParams, _ interface{}) middleware.Responder {
		ae.InitPermissionsContext(params.HTTPRequest, permissions.Management, "revocation", permissions.Create)
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Create(ae, rc, params) }, params.HTTPRequest, "", "", permissions.DefaultManagementAccess())
	})
}

func (r *RevocationRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler[*model.Revocation](ae, rc, ae.Managers.Revocation, MapRevocationToRestEntity)
}

func (r *RevocationRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler[*model.Revocation](ae, rc, ae.Managers.Revocation, MapRevocationToRestEntity)
}

// Create handles POST /revocations. It validates the submitted id against the
// revocation type, computes the expiry from the configured refresh token duration,
// and persists the revocation entry.
func (r *RevocationRouter) Create(ae *env.AppEnv, rc *response.RequestContext, params revocation.CreateRevocationParams) {
	Create(rc, rc, RevocationLinkFactory, func() (string, error) {
		entity, err := mapCreateRevocationToModel(ae, rc.Request, params.Revocation)
		if err != nil {
			return "", err
		}
		if err = ae.Managers.Revocation.Create(entity, rc.NewChangeContext()); err != nil {
			return "", err
		}
		return entity.Id, nil
	})
}

// mapCreateRevocationToModel converts a RevocationCreate REST model to a model.Revocation,
// validating the id against the revocation type and computing the server-side expiry.
func mapCreateRevocationToModel(ae *env.AppEnv, r *http.Request, create *rest_model.RevocationCreate) (*model.Revocation, error) {
	if create == nil {
		return nil, errorz.NewUnhandled(nil)
	}

	id := ""
	if create.ID != nil {
		id = *create.ID
	}

	revocationType := rest_model.RevocationTypeEnumJTI
	if create.Type != nil {
		revocationType = *create.Type
	}

	switch revocationType {
	case rest_model.RevocationTypeEnumIDENTITY:
		if _, err := ae.Managers.Identity.Read(id); err != nil {
			if boltz.IsErrNotFoundErr(err) {
				return nil, errorz.NewNotFound()
			}
			return nil, err
		}
	case rest_model.RevocationTypeEnumJTI, rest_model.RevocationTypeEnumAPISESSION:
		if _, err := uuid.Parse(id); err != nil {
			return nil, errorz.NewFieldError("must be a valid UUID v4", "id", id)
		}
	}

	expiresAt := time.Now().Add(ae.GetConfig().Edge.Oidc.RefreshTokenDuration)

	return &model.Revocation{
		BaseEntity: models.BaseEntity{
			Id:   id,
			Tags: TagsOrDefault(create.Tags),
		},
		ExpiresAt: expiresAt,
		Type:      string(revocationType),
	}, nil
}
