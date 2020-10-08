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
	"github.com/openziti/foundation/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
)

type PostureCheck struct {
	models.BaseEntity
	Name string
}

func (entity *PostureCheck) fillFrom(_ Handler, _ *bbolt.Tx, boltEntity boltz.Entity) error {
	boltPostureCheck, ok := boltEntity.(*persistence.PostureCheck)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model ca", reflect.TypeOf(boltEntity))
	}
	entity.FillCommon(boltPostureCheck)
	entity.Name = boltPostureCheck.Name
	return nil
}

func (entity *PostureCheck) toBoltEntityForCreate(tx *bbolt.Tx, handler Handler) (boltz.Entity, error) {

	boltEntity := &persistence.PostureCheck{
		BaseExtEntity: *boltz.NewExtEntity(entity.Id, entity.Tags),
		Name:          entity.Name,
	}

	return boltEntity, nil
}

func (entity *PostureCheck) toBoltEntityForUpdate(_ *bbolt.Tx, _ Handler) (boltz.Entity, error) {
	boltEntity := &persistence.PostureCheck{
		BaseExtEntity: *boltz.NewExtEntity(entity.Id, entity.Tags),
		Name:          entity.Name,
	}

	return boltEntity, nil
}

func (entity *PostureCheck) toBoltEntityForPatch(tx *bbolt.Tx, handler Handler) (boltz.Entity, error) {
	return entity.toBoltEntityForUpdate(tx, handler)
}
