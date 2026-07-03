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

package model

import (
	"time"

	"github.com/openziti/ziti/v2/controller/db"
	"github.com/openziti/ziti/v2/controller/models"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"go.etcd.io/bbolt"
)

type Revocation struct {
	models.BaseEntity
	ExpiresAt    time.Time
	Type         string
	IssuedBefore time.Time
}

// RevokesSessionIssuedAt reports whether this identity revocation applies to a
// session issued at issuedAt. A zero IssuedBefore revokes all of the identity's
// sessions; otherwise only those issued strictly before the cutoff are revoked,
// so a session authenticated after a re-enable survives a still-lingering
// revocation. This mirrors RouterDataModel.IsIdentityRevoked so the controller
// and the routers agree on the cutoff.
func (entity *Revocation) RevokesSessionIssuedAt(issuedAt time.Time) bool {
	if entity.IssuedBefore.IsZero() {
		return true
	}
	return issuedAt.Before(entity.IssuedBefore)
}

func (entity *Revocation) toBoltEntityForUpdate(tx *bbolt.Tx, env Env, _ boltz.FieldChecker) (*db.Revocation, error) {
	return entity.toBoltEntityForCreate(tx, env)
}

func (entity *Revocation) fillFrom(_ Env, _ *bbolt.Tx, boltRevocation *db.Revocation) error {
	entity.FillCommon(boltRevocation)
	entity.ExpiresAt = boltRevocation.ExpiresAt
	entity.Type = boltRevocation.Type
	entity.IssuedBefore = boltRevocation.IssuedBefore

	return nil
}

func (entity *Revocation) toBoltEntityForCreate(*bbolt.Tx, Env) (*db.Revocation, error) {
	boltEntity := &db.Revocation{
		BaseExtEntity: *boltz.NewExtEntity(entity.Id, entity.Tags),
		ExpiresAt:     entity.ExpiresAt,
		Type:          entity.Type,
		IssuedBefore:  entity.IssuedBefore,
	}

	return boltEntity, nil
}
