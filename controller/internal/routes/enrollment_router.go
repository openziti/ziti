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
	"errors"
	"time"

	"github.com/go-openapi/runtime/middleware"
	"github.com/openziti/edge-api/rest_management_api_server/operations/enrollment"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/v2/controller/env"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/models"
	"github.com/openziti/ziti/v2/controller/permissions"
	"github.com/openziti/ziti/v2/controller/response"
	"github.com/openziti/ziti/v2/controller/storage/ast"
)

func init() {
	r := NewEnrollmentRouter()
	env.AddRouter(r)
}

type EnrollmentRouter struct {
	BasePath string
}

func NewEnrollmentRouter() *EnrollmentRouter {
	return &EnrollmentRouter{
		BasePath: "/" + EntityNameEnrollment,
	}
}

func (r *EnrollmentRouter) Register(ae *env.AppEnv) {
	ae.ManagementApi.EnrollmentDeleteEnrollmentHandler = enrollment.DeleteEnrollmentHandlerFunc(func(params enrollment.DeleteEnrollmentParams, _ interface{}) middleware.Responder {
		ae.InitPermissionsContext(params.HTTPRequest, permissions.Management, "enrollment", permissions.Delete)
		return ae.IsAllowed(r.Delete, params.HTTPRequest, params.ID, "", permissions.DefaultManagementAccess())
	})

	ae.ManagementApi.EnrollmentDetailEnrollmentHandler = enrollment.DetailEnrollmentHandlerFunc(func(params enrollment.DetailEnrollmentParams, _ interface{}) middleware.Responder {
		ae.InitPermissionsContext(params.HTTPRequest, permissions.Management, "enrollment", permissions.Read)
		return ae.IsAllowed(r.Detail, params.HTTPRequest, params.ID, "", permissions.DefaultManagementAccess())
	})

	ae.ManagementApi.EnrollmentListEnrollmentsHandler = enrollment.ListEnrollmentsHandlerFunc(func(params enrollment.ListEnrollmentsParams, _ interface{}) middleware.Responder {
		ae.InitPermissionsContext(params.HTTPRequest, permissions.Management, "enrollment", permissions.Read)
		return ae.IsAllowed(r.List, params.HTTPRequest, "", "", permissions.DefaultManagementAccess())
	})

	ae.ManagementApi.EnrollmentRefreshEnrollmentHandler = enrollment.RefreshEnrollmentHandlerFunc(func(params enrollment.RefreshEnrollmentParams, _ interface{}) middleware.Responder {
		ae.InitPermissionsContext(params.HTTPRequest, permissions.Management, "enrollment", permissions.Update)
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.Refresh(ae, rc, params)
		}, params.HTTPRequest, params.ID, "", permissions.DefaultManagementAccess())
	})

	ae.ManagementApi.EnrollmentCreateEnrollmentHandler = enrollment.CreateEnrollmentHandlerFunc(func(params enrollment.CreateEnrollmentParams, _ interface{}) middleware.Responder {
		ae.InitPermissionsContext(params.HTTPRequest, permissions.Management, "enrollment", permissions.Create)
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.Create(ae, rc, params)
		}, params.HTTPRequest, "", "", permissions.DefaultManagementAccess())
	})
}

func (r *EnrollmentRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	lister := ae.Managers.Enrollment
	ListWithQueryF[*model.Enrollment](ae, rc, lister, MapEnrollmentToRestEntity, func(query ast.Query) (*models.EntityListResult[*model.Enrollment], error) {
		// Enrollments expose the one-time-token used to enroll as their target identity, so a
		// non-admin must not be able to see enrollments belonging to admin identities.
		if !rc.HasPermission(permissions.AdminPermission) {
			scopeQuery, err := ast.Parse(lister.GetSymbolTypes(), "not (identity.isAdmin = true)")
			if err != nil {
				return nil, err
			}
			query.SetPredicate(ast.NewAndExprNode(query.GetPredicate(), scopeQuery.GetPredicate()))
		}
		return lister.BasePreparedList(query)
	})
}

func (r *EnrollmentRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	if !r.allowNonAdminAccessToEnrollment(ae, rc) {
		return
	}
	DetailWithHandler[*model.Enrollment](ae, rc, ae.Managers.Enrollment, MapEnrollmentToRestEntity)
}

func (r *EnrollmentRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	if !r.allowNonAdminAccessToEnrollment(ae, rc) {
		return
	}
	DeleteWithHandler(rc, ae.Managers.Enrollment)
}

func (r *EnrollmentRouter) Refresh(ae *env.AppEnv, rc *response.RequestContext, params enrollment.RefreshEnrollmentParams) {
	if !r.allowNonAdminAccessToEnrollment(ae, rc) {
		return
	}

	id, err := rc.GetEntityId()

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	if id == "" {
		rc.RespondWithNotFound()
		return
	}

	if err := ae.Managers.Enrollment.RefreshJwt(id, time.Time(*params.Refresh.ExpiresAt), rc.NewChangeContext()); err != nil {
		if fe, ok := err.(*errorz.FieldError); ok {
			rc.RespondWithFieldError(fe)
			return
		}
		rc.RespondWithError(err)
		return
	}

	rc.RespondWithEmptyOk()
}

func (r *EnrollmentRouter) Create(ae *env.AppEnv, rc *response.RequestContext, params enrollment.CreateEnrollmentParams) {
	if !r.allowNonAdminEnrollmentForIdentity(ae, rc, stringz.OrEmpty(params.Enrollment.IdentityID)) {
		return
	}

	Create(rc, rc, EnrollmentLinkFactory, func() (string, error) {
		return MapCreate(ae.Managers.Enrollment.Create, MapCreateEnrollmentToModel(params.Enrollment), rc)
	})

}

// allowNonAdminEnrollmentForIdentity ensures that a non-admin caller is not creating an enrollment
// that targets an admin identity. An enrollment mints the one-time-token used to enroll as its
// target identity, so allowing this would let a non-admin escalate to admin. It responds with an
// unauthorized error and returns false when the operation should be denied.
func (r *EnrollmentRouter) allowNonAdminEnrollmentForIdentity(ae *env.AppEnv, rc *response.RequestContext, identityId string) bool {
	if rc.HasPermission(permissions.AdminPermission) {
		return true
	}

	if identityId != "" {
		if identity, _ := ae.Managers.Identity.Read(identityId); identity != nil && identity.IsAdmin {
			rc.RespondWithError(nonAdminNotAllowedError(errors.New("non-admins may not manage enrollments for admin identities")))
			return false
		}
	}

	return true
}

// allowNonAdminAccessToEnrollment ensures that a non-admin caller is not reading, refreshing, or
// deleting an enrollment that belongs to an admin identity. The enrollment carries the live token
// and JWT used to enroll as its target identity, so exposing or refreshing an admin identity's
// enrollment would permit privilege escalation. It responds with an unauthorized error and returns
// false when the operation should be denied. When the enrollment cannot be loaded, it allows the
// request to proceed so the downstream handler can produce the appropriate not-found response.
func (r *EnrollmentRouter) allowNonAdminAccessToEnrollment(ae *env.AppEnv, rc *response.RequestContext) bool {
	if rc.HasPermission(permissions.AdminPermission) {
		return true
	}

	id, err := rc.GetEntityId()
	if err != nil {
		rc.RespondWithError(err)
		return false
	}

	enrollmentEntity, err := ae.Managers.Enrollment.Read(id)
	if err != nil || enrollmentEntity == nil || enrollmentEntity.IdentityId == nil {
		return true
	}

	return r.allowNonAdminEnrollmentForIdentity(ae, rc, *enrollmentEntity.IdentityId)
}

func MapCreateEnrollmentToModel(create *rest_model.EnrollmentCreate) *model.Enrollment {
	ret := &model.Enrollment{
		Method:     *create.Method,
		IdentityId: create.IdentityID,
		ExpiresAt:  (*time.Time)(create.ExpiresAt),
		CaId:       create.CaID,
		Username:   create.Username,
	}

	return ret
}
