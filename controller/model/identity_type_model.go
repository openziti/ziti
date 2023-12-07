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
)

type IdentityType struct {
	models.BaseEntity
	Name string `json:"name"`
}

func (entity *IdentityType) toBoltEntity() (*db.IdentityType, error) {
	return &db.IdentityType{
		Name:          entity.Name,
		BaseExtEntity: *boltz.NewExtEntity(entity.Id, entity.Tags),
	}, nil
}

func (entity *IdentityType) toBoltEntityForCreate(*bbolt.Tx, Env) (*db.IdentityType, error) {
	return entity.toBoltEntity()
}

func (entity *IdentityType) toBoltEntityForUpdate(*bbolt.Tx, Env, boltz.FieldChecker) (*db.IdentityType, error) {
	return entity.toBoltEntity()
}

func (entity *IdentityType) fillFrom(_ Env, _ *bbolt.Tx, boltIdentityType *db.IdentityType) error {
	entity.FillCommon(boltIdentityType)
	entity.Name = boltIdentityType.Name
	return nil
}
