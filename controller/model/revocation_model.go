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
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/models"
	"go.etcd.io/bbolt"
	"time"
)

type Revocation struct {
	models.BaseEntity
	ExpiresAt time.Time
}

func (entity *Revocation) toBoltEntityForUpdate(tx *bbolt.Tx, env Env, checker boltz.FieldChecker) (*db.Revocation, error) {
	return entity.toBoltEntityForCreate(tx, env)
}

func (entity *Revocation) fillFrom(_ Env, _ *bbolt.Tx, boltRevocation *db.Revocation) error {
	entity.FillCommon(boltRevocation)
	entity.ExpiresAt = boltRevocation.ExpiresAt

	return nil
}

func (entity *Revocation) toBoltEntityForCreate(*bbolt.Tx, Env) (*db.Revocation, error) {
	boltEntity := &db.Revocation{
		BaseExtEntity: *boltz.NewExtEntity(entity.Id, entity.Tags),
		ExpiresAt:     entity.ExpiresAt,
	}

	return boltEntity, nil
}
