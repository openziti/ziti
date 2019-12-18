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
	"github.com/netfoundry/ziti-edge/controller/response"
	"github.com/netfoundry/ziti-edge/migration"

	"fmt"
	"github.com/michaelquigley/pfxlog"
)

func init() {
	r := NewEnrollmentRouter()
	env.AddRouter(r)
}

type EnrollmentRouter struct {
	BasePath string
	IdType   response.IdType
}

func (ir *EnrollmentRouter) ToApiDetailEntity(ae *env.AppEnv, rc *response.RequestContext, e migration.BaseDbModel) (BaseApiEntity, error) {
	return ir.ToApiListEntity(ae, rc, e)
}

func (ir *EnrollmentRouter) ToApiListEntity(ae *env.AppEnv, rc *response.RequestContext, e migration.BaseDbModel) (BaseApiEntity, error) {
	i, ok := e.(*migration.Enrollment)

	if !ok {
		err := fmt.Errorf("entity is not a cluster \"%s\"", e.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	al, err := NewEnrollmentApiList(ae, i)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return al, nil
}

func NewEnrollmentRouter() *EnrollmentRouter {
	return &EnrollmentRouter{
		BasePath: "/" + EntityNameEnrollment,
		IdType:   response.IdTypeUuid,
	}
}

func (ir *EnrollmentRouter) Register(ae *env.AppEnv) {
	registerReadDeleteOnlyRouter(ae, ae.RootRouter, ir.BasePath, ir, permissions.IsAdmin())
}

func (ir *EnrollmentRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler(ae, rc, ae.Handlers.Enrollment, MapEnrollmentToApiEntity)
}

func (ir *EnrollmentRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Handlers.Identity, MapEnrollmentToApiEntity, ir.IdType)
}

func (ir *EnrollmentRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ir.IdType, ae.Handlers.Enrollment)
}
