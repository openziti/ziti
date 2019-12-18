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
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/env"
	"github.com/netfoundry/ziti-edge/controller/model"
	"github.com/netfoundry/ziti-edge/controller/response"
	"github.com/netfoundry/ziti-edge/migration"
	"time"
)

const EntityNameEnrollment = "enrollments"

type EnrollmentApiList struct {
	*env.BaseApi
	Token     *string       `json:"token"`
	Method    *string       `json:"method"`
	ExpiresAt *time.Time    `json:"expiresAt"`
	Identity  *EntityApiRef `json:"identity"`
	Details   interface{}   `json:"details"`
}

func (e *EnrollmentApiList) GetSelfLink() *response.Link {
	return e.BuildSelfLink(e.Id)
}

func (EnrollmentApiList) BuildSelfLink(id string) *response.Link {
	return response.NewLink(fmt.Sprintf("./%s/%s", EntityNameEnrollment, id))
}

func (e *EnrollmentApiList) PopulateLinks() {
	if e.Links == nil {
		self := e.GetSelfLink()
		e.Links = &response.Links{
			EntityNameSelf: self,
		}
	}
}

func (e *EnrollmentApiList) ToEntityApiRef() *EntityApiRef {
	e.PopulateLinks()
	return &EntityApiRef{
		Entity: EntityNameEnrollment,
		Name:   e.Method,
		Id:     e.Id,
		Links:  e.Links,
	}
}

func NewEnrollmentApiList(ae *env.AppEnv, i *migration.Enrollment) (*EnrollmentApiList, error) {
	baseApi := env.FromBaseDbEntity(&i.BaseDbEntity)

	ret := &EnrollmentApiList{
		BaseApi:   baseApi,
		Method:    i.Method,
		Token:     i.Token,
		ExpiresAt: i.ExpiresAt,
		Identity:  nil,
		Details:   nil,
	}

	return ret, nil
}

func MapEnrollmentToApiEntity(appEnv *env.AppEnv, context *response.RequestContext, entity model.BaseModelEntity) (BaseApiEntity, error) {
	enrollment, ok := entity.(*model.Enrollment)

	if !ok {
		err := fmt.Errorf("entity is not an enrollment \"%s\"", entity.GetId())
		pfxlog.Logger().Error(err)
		return nil, err
	}

	al, err := MapToEnrollmentApiList(appEnv, enrollment)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", entity.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return al, nil
}

func MapToEnrollmentApiList(ae *env.AppEnv, enrollment *model.Enrollment) (*EnrollmentApiList, error) {

	identity, err := ae.Handlers.Identity.HandleRead(enrollment.IdentityId)

	if err != nil {
		return nil, err
	}

	ret := &EnrollmentApiList{
		BaseApi:   env.FromBaseModelEntity(enrollment),
		Token:     &enrollment.Token,
		Method:    &enrollment.Method,
		ExpiresAt: enrollment.ExpiresAt,
		Identity:  NewIdentityEntityRef(identity),
	}

	ret.PopulateLinks()

	return ret, nil
}
