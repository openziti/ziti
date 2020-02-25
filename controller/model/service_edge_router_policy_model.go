/*
	Copyright 2019 NetFoundry, Inc.

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

type ServiceEdgeRouterPolicy struct {
	BaseModelEntityImpl
	Name            string
	Semantic        string
	ServiceRoles    []string
	EdgeRouterRoles []string
}

func (entity *ServiceEdgeRouterPolicy) toBoltEntityForCreate(_ *bbolt.Tx, _ Handler) (persistence.BaseEdgeEntity, error) {
	return &persistence.ServiceEdgeRouterPolicy{
		BaseEdgeEntityImpl: *persistence.NewBaseEdgeEntity(entity.Id, entity.Tags),
		Name:               entity.Name,
		Semantic:           entity.Semantic,
		ServiceRoles:       entity.ServiceRoles,
		EdgeRouterRoles:    entity.EdgeRouterRoles,
	}, nil
}

func (entity *ServiceEdgeRouterPolicy) toBoltEntityForUpdate(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	return entity.toBoltEntityForCreate(tx, handler)
}

func (entity *ServiceEdgeRouterPolicy) toBoltEntityForPatch(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	return entity.toBoltEntityForCreate(tx, handler)
}

func (entity *ServiceEdgeRouterPolicy) fillFrom(_ Handler, _ *bbolt.Tx, boltEntity boltz.BaseEntity) error {
	boltServiceEdgeRouterPolicy, ok := boltEntity.(*persistence.ServiceEdgeRouterPolicy)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model edge router policy", reflect.TypeOf(boltEntity))
	}
	entity.fillCommon(boltServiceEdgeRouterPolicy)
	entity.Name = boltServiceEdgeRouterPolicy.Name
	entity.Semantic = boltServiceEdgeRouterPolicy.Semantic
	entity.EdgeRouterRoles = boltServiceEdgeRouterPolicy.EdgeRouterRoles
	entity.ServiceRoles = boltServiceEdgeRouterPolicy.ServiceRoles
	return nil
}
