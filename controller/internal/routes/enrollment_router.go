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
	r.checkNonAdminAccessToEnrollment(ae, rc, func() {
		DetailWithHandler[*model.Enrollment](ae, rc, ae.Managers.Enrollment, MapEnrollmentToRestEntity)
	})
}

func (r *EnrollmentRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	r.checkNonAdminAccessToEnrollment(ae, rc, func() {
		DeleteWithHandler(rc, ae.Managers.Enrollment)
	})
}

func (r *EnrollmentRouter) Refresh(ae *env.AppEnv, rc *response.RequestContext, params enrollment.RefreshEnrollmentParams) {
	r.checkNonAdminAccessToEnrollment(ae, rc, func() {
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
	})
}

func (r *EnrollmentRouter) Create(ae *env.AppEnv, rc *response.RequestContext, params enrollment.CreateEnrollmentParams) {
	r.checkNonAdminEnrollmentForIdentity(ae, rc, stringz.OrEmpty(params.Enrollment.IdentityID), func() {
		Create(rc, rc, EnrollmentLinkFactory, func() (string, error) {
			return MapCreate(ae.Managers.Enrollment.Create, MapCreateEnrollmentToModel(params.Enrollment), rc)
		})
	})
}

// checkNonAdminEnrollmentForIdentity ensures that a non-admin caller is not creating an enrollment
// that targets an admin identity. An enrollment mints the one-time-token used to enroll as its
// target identity, so allowing this would let a non-admin escalate to admin. When access is granted
// it invokes handler; otherwise it writes the appropriate error response and does not call handler.
func (r *EnrollmentRouter) checkNonAdminEnrollmentForIdentity(ae *env.AppEnv, rc *response.RequestContext, identityId string, handler func()) {
	if rc.HasPermission(permissions.AdminPermission) {
		handler()
		return
	}

	if identityId != "" {
		identity, err := ae.Managers.Identity.Read(identityId)
		if err != nil {
			rc.RespondWithError(err)
			return
		}
		if identity != nil && identity.IsAdmin {
			rc.RespondWithError(nonAdminNotAllowedError(errors.New("non-admins may not manage enrollments for admin identities")))
			return
		}
	}

	handler()
}

// checkNonAdminAccessToEnrollment ensures that a non-admin caller is not reading, refreshing, or
// deleting an enrollment that belongs to an admin identity. The enrollment carries the live token
// and JWT used to enroll as its target identity, so exposing or refreshing an admin identity's
// enrollment would permit privilege escalation. When access is granted it invokes handler;
// otherwise it writes the appropriate error response and does not call handler. When the enrollment
// cannot be loaded it denies the request rather than relying on a downstream handler to do so.
func (r *EnrollmentRouter) checkNonAdminAccessToEnrollment(ae *env.AppEnv, rc *response.RequestContext, handler func()) {
	if rc.HasPermission(permissions.AdminPermission) {
		handler()
		return
	}

	id, err := rc.GetEntityId()
	if err != nil {
		rc.RespondWithError(err)
		return
	}

	enrollmentEntity, err := ae.Managers.Enrollment.Read(id)
	if err != nil {
		rc.RespondWithError(err)
		return
	}
	if enrollmentEntity == nil {
		rc.RespondWithNotFound()
		return
	}
	if enrollmentEntity.IdentityId == nil {
		// an enrollment not tied to an identity has no admin to escalate to
		handler()
		return
	}

	r.checkNonAdminEnrollmentForIdentity(ae, rc, *enrollmentEntity.IdentityId, handler)
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
