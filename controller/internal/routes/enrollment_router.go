/*
	Copyright NetFoundry, Inc.

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
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/internal/permissions"
	"github.com/openziti/edge/controller/response"
)

func init() {
	r := NewEnrollmentRouter()
	env.AddRouter(r)
}

type EnrollmentRouter struct {
	BasePath string
	IdType   response.IdType
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
