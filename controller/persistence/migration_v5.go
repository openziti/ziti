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

package persistence

import (
	"github.com/openziti/foundation/storage/boltz"
)

func (m *Migrations) createEnrollmentsForEdgeRouters(step *boltz.MigrationStep) {
	edgeRouterIds, _, err := m.stores.EdgeRouter.QueryIds(step.Ctx.Tx(), "true")
	step.SetError(err)

	for _, edgeRouterId := range edgeRouterIds {
		edgeRouter, err := m.stores.EdgeRouter.LoadOneById(step.Ctx.Tx(), edgeRouterId)
		if step.SetError(err) {
			return
		}
		if edgeRouter.EnrollmentJwt == nil {
			continue
		}

		enrollment := &Enrollment{
			Token:        *edgeRouter.EnrollmentToken,
			Method:       MethodEnrollEdgeRouterOtt,
			EdgeRouterId: &edgeRouter.Id,
			ExpiresAt:    edgeRouter.EnrollmentExpiresAt,
			IssuedAt:     edgeRouter.EnrollmentCreatedAt,
			Jwt:          *edgeRouter.EnrollmentJwt,
		}

		step.SetError(m.stores.Enrollment.Create(step.Ctx, enrollment))

		edgeRouter.EnrollmentJwt = nil
		edgeRouter.EnrollmentCreatedAt = nil
		edgeRouter.EnrollmentExpiresAt = nil
		edgeRouter.EnrollmentToken = nil

		step.SetError(m.stores.EdgeRouter.Update(step.Ctx, edgeRouter, nil))
	}
}
