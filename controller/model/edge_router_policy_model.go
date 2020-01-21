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

type EdgeRouterPolicy struct {
	BaseModelEntityImpl
	Name            string
	Semantic        string
	IdentityRoles   []string
	EdgeRouterRoles []string
}

func (entity *EdgeRouterPolicy) ToBoltEntityForCreate(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	return &persistence.EdgeRouterPolicy{
		BaseEdgeEntityImpl: *persistence.NewBaseEdgeEntity(entity.Id, entity.Tags),
		Name:               entity.Name,
		Semantic:           entity.Semantic,
		IdentityRoles:      entity.IdentityRoles,
		EdgeRouterRoles:    entity.EdgeRouterRoles,
	}, nil
}

func (entity *EdgeRouterPolicy) ToBoltEntityForUpdate(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	return entity.ToBoltEntityForCreate(tx, handler)
}

func (entity *EdgeRouterPolicy) ToBoltEntityForPatch(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	return entity.ToBoltEntityForCreate(tx, handler)
}

func (entity *EdgeRouterPolicy) FillFrom(handler Handler, tx *bbolt.Tx, boltEntity boltz.BaseEntity) error {
	boltEdgeRouterPolicy, ok := boltEntity.(*persistence.EdgeRouterPolicy)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model edge router policy", reflect.TypeOf(boltEntity))
	}
	entity.fillCommon(boltEdgeRouterPolicy)
	entity.Name = boltEdgeRouterPolicy.Name
	entity.Semantic = boltEdgeRouterPolicy.Semantic
	entity.EdgeRouterRoles = boltEdgeRouterPolicy.EdgeRouterRoles
	entity.IdentityRoles = boltEdgeRouterPolicy.IdentityRoles
	return nil
}
