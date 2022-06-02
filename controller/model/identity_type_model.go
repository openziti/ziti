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

package model

import (
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
)

const (
	IdentityTypeUser = "User"
)

type IdentityType struct {
	models.BaseEntity
	Name string `json:"name"`
}

func (entity *IdentityType) toBoltEntity() (boltz.Entity, error) {
	return &persistence.IdentityType{
		Name:          entity.Name,
		BaseExtEntity: *boltz.NewExtEntity(entity.Id, entity.Tags),
	}, nil
}

func (entity *IdentityType) toBoltEntityForCreate(*bbolt.Tx, EntityManager) (boltz.Entity, error) {
	return entity.toBoltEntity()
}

func (entity *IdentityType) toBoltEntityForUpdate(*bbolt.Tx, EntityManager) (boltz.Entity, error) {
	return entity.toBoltEntity()
}

func (entity *IdentityType) toBoltEntityForPatch(*bbolt.Tx, EntityManager, boltz.FieldChecker) (boltz.Entity, error) {
	return entity.toBoltEntity()
}

func (entity *IdentityType) fillFrom(_ EntityManager, _ *bbolt.Tx, boltEntity boltz.Entity) error {
	boltIdentityType, ok := boltEntity.(*persistence.IdentityType)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model IdentityType", reflect.TypeOf(boltEntity))
	}
	entity.FillCommon(boltIdentityType)
	entity.Name = boltIdentityType.Name
	return nil
}
