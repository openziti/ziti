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

package db

import (
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
)

func (m *Migrations) migrateIdentityTypesToDefault(step *boltz.MigrationStep) {
	step.SetError(m.stores.IdentityType.Create(step.Ctx, &IdentityType{
		BaseExtEntity: *boltz.NewExtEntity(DefaultIdentityType, nil),
		Name:          DefaultIdentityType,
	}))

	cursor := m.stores.Identity.IterateIds(step.Ctx.Tx(), ast.BoolNodeTrue)
	for cursor.IsValid() {
		id := string(cursor.Current())
		if identityEntity, err := m.stores.Identity.LoadById(step.Ctx.Tx(), id); !step.SetError(err) {
			if identityEntity.IdentityTypeId != RouterIdentityType {
				identityEntity.IdentityTypeId = DefaultIdentityType
				err = m.stores.Identity.Update(step.Ctx, identityEntity, boltz.MapFieldChecker{
					FieldIdentityType: struct{}{},
					"identityTypeId":  struct{}{},
				})
				step.SetError(err)
			}
		}
		cursor.Next()
	}

	step.SetError(m.stores.IdentityType.DeleteById(step.Ctx, "User"))
	step.SetError(m.stores.IdentityType.DeleteById(step.Ctx, "Service"))
	step.SetError(m.stores.IdentityType.DeleteById(step.Ctx, "Device"))
}
