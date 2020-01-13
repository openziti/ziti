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

type Identity struct {
	BaseModelEntityImpl
	Name           string
	IdentityTypeId string
	IsDefaultAdmin bool
	IsAdmin        bool
	RoleAttributes []string
}

func (entity *Identity) ToBoltEntityForCreate(_ *bbolt.Tx, _ Handler) (persistence.BaseEdgeEntity, error) {
	edgeService := &persistence.Identity{
		BaseEdgeEntityImpl: *persistence.NewBaseEdgeEntity(entity.Id, entity.Tags),
		Name:               entity.Name,
		IdentityTypeId:     entity.IdentityTypeId,
		IsDefaultAdmin:     entity.IsDefaultAdmin,
		IsAdmin:            entity.IsAdmin,
		RoleAttributes:     entity.RoleAttributes,
	}

	return edgeService, nil
}

func (entity *Identity) ToBoltEntityForUpdate(*bbolt.Tx, Handler) (persistence.BaseEdgeEntity, error) {
	return &persistence.Identity{
		Name:               entity.Name,
		IdentityTypeId:     entity.IdentityTypeId,
		BaseEdgeEntityImpl: *persistence.NewBaseEdgeEntity(entity.Id, entity.Tags),
		RoleAttributes:     entity.RoleAttributes,
	}, nil
}

func (entity *Identity) ToBoltEntityForPatch(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	return entity.ToBoltEntityForUpdate(tx, handler)
}

func (entity *Identity) FillFrom(_ Handler, _ *bbolt.Tx, boltEntity boltz.BaseEntity) error {
	boltIdentity, ok := boltEntity.(*persistence.Identity)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model identity", reflect.TypeOf(boltEntity))
	}
	entity.fillCommon(boltIdentity)
	entity.Name = boltIdentity.Name
	entity.IdentityTypeId = boltIdentity.IdentityTypeId
	entity.IsDefaultAdmin = boltIdentity.IsDefaultAdmin
	entity.IsAdmin = boltIdentity.IsAdmin
	entity.RoleAttributes = boltIdentity.RoleAttributes

	return nil
}
