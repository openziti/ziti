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
	"fmt"
	"github.com/openziti/foundation/storage/boltz"
)

func (m *Migrations) fixIdentityBuckets(step *boltz.MigrationStep) {
	identityIds, _, err := m.stores.Identity.QueryIds(step.Ctx.Tx(), "true")
	step.SetError(err)

	for _, identityId := range identityIds {
		identity, err := m.stores.Identity.LoadOneById(step.Ctx.Tx(), identityId)

		if step.SetError(err) {
			return
		}

		authIds, _, err := m.stores.Authenticator.QueryIds(step.Ctx.Tx(), fmt.Sprintf(`identity="%s"`, identity.Id))
		if step.SetError(err) {
			return
		}

		if len(authIds) != len(identity.Authenticators) {
			for _, authId := range authIds {
				authenticator, err := m.stores.Authenticator.LoadOneById(step.Ctx.Tx(), authId)
				if step.SetError(err) {
					return
				}

				err = m.stores.Authenticator.DeleteById(step.Ctx, authenticator.Id)
				if step.SetError(err) {
					return
				}

				err = m.stores.Authenticator.Create(step.Ctx, authenticator)
				if step.SetError(err) {
					return
				}
			}
		}

		enrollIds, _, err := m.stores.Enrollment.QueryIds(step.Ctx.Tx(), fmt.Sprintf(`identity="%s"`, identity.Id))
		if step.SetError(err) {
			return
		}

		if len(enrollIds) != len(identity.Enrollments) {
			for _, enrolId := range enrollIds {
				err = m.stores.Enrollment.DeleteById(step.Ctx, enrolId)
				if step.SetError(err) {
					return
				}
			}
		}
	}
}
