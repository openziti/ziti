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

type ServiceEdgeRouterPolicy struct {
	models.BaseEntity
	Name            string
	Semantic        string
	ServiceRoles    []string
	EdgeRouterRoles []string
}

func (entity *ServiceEdgeRouterPolicy) toBoltEntity() (boltz.Entity, error) {
	return &persistence.ServiceEdgeRouterPolicy{
		BaseExtEntity:   *boltz.NewExtEntity(entity.Id, entity.Tags),
		Name:            entity.Name,
		Semantic:        entity.Semantic,
		ServiceRoles:    entity.ServiceRoles,
		EdgeRouterRoles: entity.EdgeRouterRoles,
	}, nil
}

func (entity *ServiceEdgeRouterPolicy) toBoltEntityForCreate(*bbolt.Tx, EntityManager) (boltz.Entity, error) {
	return entity.toBoltEntity()
}

func (entity *ServiceEdgeRouterPolicy) toBoltEntityForUpdate(*bbolt.Tx, EntityManager) (boltz.Entity, error) {
	return entity.toBoltEntity()
}

func (entity *ServiceEdgeRouterPolicy) toBoltEntityForPatch(*bbolt.Tx, EntityManager, boltz.FieldChecker) (boltz.Entity, error) {
	return entity.toBoltEntity()
}

func (entity *ServiceEdgeRouterPolicy) fillFrom(_ EntityManager, _ *bbolt.Tx, boltEntity boltz.Entity) error {
	boltServiceEdgeRouterPolicy, ok := boltEntity.(*persistence.ServiceEdgeRouterPolicy)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model edge router policy", reflect.TypeOf(boltEntity))
	}
	entity.FillCommon(boltServiceEdgeRouterPolicy)
	entity.Name = boltServiceEdgeRouterPolicy.Name
	entity.Semantic = boltServiceEdgeRouterPolicy.Semantic
	entity.EdgeRouterRoles = boltServiceEdgeRouterPolicy.EdgeRouterRoles
	entity.ServiceRoles = boltServiceEdgeRouterPolicy.ServiceRoles
	return nil
}
