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

package model

import (
	"github.com/netfoundry/ziti-edge/controller/persistence"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
)

const (
	IdentityTypeUser = "User"
)

type IdentityType struct {
	BaseModelEntityImpl
	Name string `json:"name"`
}

func (entity *IdentityType) toBoltEntityForCreate(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	return &persistence.IdentityType{
		Name:               entity.Name,
		BaseEdgeEntityImpl: *persistence.NewBaseEdgeEntity(entity.Id, entity.Tags),
	}, nil
}

func (entity *IdentityType) toBoltEntityForUpdate(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	return entity.toBoltEntityForCreate(tx, handler)
}

func (entity *IdentityType) toBoltEntityForPatch(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	return entity.toBoltEntityForCreate(tx, handler)
}

func (entity *IdentityType) fillFrom(handler Handler, tx *bbolt.Tx, boltEntity boltz.BaseEntity) error {
	boltIdentityType, ok := boltEntity.(*persistence.IdentityType)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model IdentityType", reflect.TypeOf(boltEntity))
	}
	entity.fillCommon(boltIdentityType)
	entity.Name = boltIdentityType.Name
	return nil
}
