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
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/response"
	"github.com/openziti/foundation/v2/stringz"
)

const EntityNameEnrollment = "enrollments"

var EnrollmentLinkFactory = NewBasicLinkFactory(EntityNameEnrollment)

func MapEnrollmentToRestEntity(ae *env.AppEnv, _ *response.RequestContext, enrollment *model.Enrollment) (interface{}, error) {
	return MapEnrollmentToRestModel(ae, enrollment)
}

func MapEnrollmentToRestModel(ae *env.AppEnv, enrollment *model.Enrollment) (*rest_model.EnrollmentDetail, error) {
	expiresAt := strfmt.DateTime(*enrollment.ExpiresAt)
	ret := &rest_model.EnrollmentDetail{
		BaseEntity:      BaseEntityToRestModel(enrollment, EnrollmentLinkFactory),
		EdgeRouterID:    stringz.OrEmpty(enrollment.EdgeRouterId),
		ExpiresAt:       &expiresAt,
		IdentityID:      stringz.OrEmpty(enrollment.IdentityId),
		Method:          &enrollment.Method,
		Token:           &enrollment.Token,
		TransitRouterID: stringz.OrEmpty(enrollment.TransitRouterId),
		Username:        "",
		JWT:             enrollment.Jwt,
		CaID:            enrollment.CaId,
	}

	if enrollment.IdentityId != nil {
		identity, err := ae.Managers.Identity.Read(*enrollment.IdentityId)
		if err != nil {
			return nil, err
		}
		ret.Identity = ToEntityRef(identity.Name, identity, IdentityLinkFactory)
	}

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
