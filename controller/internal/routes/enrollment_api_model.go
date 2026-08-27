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
	"github.com/go-openapi/strfmt"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/v2/controller/env"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/permissions"
	"github.com/openziti/ziti/v2/controller/response"
)

const EntityNameEnrollment = "enrollments"

var EnrollmentLinkFactory = NewBasicLinkFactory(EntityNameEnrollment)

// mustWithholdEnrollmentSecrets reports whether the live enrollment token and JWT must be omitted
// when rendering an enrollment owned by the given identity. The token and JWT are the credentials
// used to enroll as that identity: redeeming one mints an authenticator on it. A caller that is not
// an admin must therefore never see them for an admin identity, or it can escalate to admin.
func mustWithholdEnrollmentSecrets(rc *response.RequestContext, owner *model.Identity) bool {
	if owner == nil || !owner.IsAdmin {
		return false
	}
	return rc == nil || !rc.HasPermission(permissions.AdminPermission)
}

func MapEnrollmentToRestEntity(ae *env.AppEnv, rc *response.RequestContext, enrollment *model.Enrollment) (interface{}, error) {
	return MapEnrollmentToRestModel(ae, rc, enrollment)
}

// MapEnrollmentToRestModel renders an enrollment for the API. The token and JWT of an enrollment
// belonging to an admin identity are withheld from callers that are not admins, since redeeming
// either yields a credential on that admin identity.
func MapEnrollmentToRestModel(ae *env.AppEnv, rc *response.RequestContext, enrollment *model.Enrollment) (*rest_model.EnrollmentDetail, error) {
	expiresAt := strfmt.DateTime(*enrollment.ExpiresAt)
	token := enrollment.Token
	jwt := enrollment.Jwt

	ret := &rest_model.EnrollmentDetail{
		BaseEntity:      BaseEntityToRestModel(enrollment, EnrollmentLinkFactory),
		EdgeRouterID:    stringz.OrEmpty(enrollment.EdgeRouterId),
		ExpiresAt:       &expiresAt,
		IdentityID:      stringz.OrEmpty(enrollment.IdentityId),
		Method:          &enrollment.Method,
		TransitRouterID: stringz.OrEmpty(enrollment.TransitRouterId),
		Username:        "",
		CaID:            enrollment.CaId,
	}

	if enrollment.IdentityId != nil {
		identity, err := ae.Managers.Identity.Read(*enrollment.IdentityId)
		if err != nil {
			return nil, err
		}
		ret.Identity = ToEntityRef(identity.Name, identity, IdentityLinkFactory)

		if mustWithholdEnrollmentSecrets(rc, identity) {
			token = ""
			jwt = ""
		}
	}

	ret.Token = &token
	ret.JWT = jwt

	if enrollment.EdgeRouterId != nil {
		edgeRouter, err := ae.Managers.EdgeRouter.Read(*enrollment.EdgeRouterId)
		if err != nil {
			return nil, err
		}
		ret.EdgeRouter = ToEntityRef(edgeRouter.Name, edgeRouter, EdgeRouterLinkFactory)
	}

	if enrollment.TransitRouterId != nil {
		transitRouter, err := ae.Managers.TransitRouter.Read(*enrollment.TransitRouterId)
		if err != nil {
			return nil, err
		}
		ret.TransitRouter = ToEntityRef(transitRouter.Name, transitRouter, TransitRouterLinkFactory)
	}

	return ret, nil
}
