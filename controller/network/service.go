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

package network

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/orcaman/concurrent-map"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
)

type Service struct {
	models.BaseEntity
	Name               string
	TerminatorStrategy string
	Terminators        []*Terminator
}

func (entity *Service) fillFrom(ctrl Controller, tx *bbolt.Tx, boltEntity boltz.Entity) error {
	boltService, ok := boltEntity.(*db.Service)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model service", reflect.TypeOf(boltEntity))
	}
	entity.Name = boltService.Name
	entity.TerminatorStrategy = boltService.TerminatorStrategy
	entity.FillCommon(boltService)

	terminatorIds := ctrl.getControllers().stores.Service.GetRelatedEntitiesIdList(tx, entity.Id, db.EntityTypeTerminators)
	for _, terminatorId := range terminatorIds {
		if terminator, _ := ctrl.getControllers().Terminators.readInTx(tx, terminatorId); terminator != nil {
			entity.Terminators = append(entity.Terminators, terminator)
		}
	}

	return nil
}

func (entity *Service) toBolt() *db.Service {
	return &db.Service{
		BaseExtEntity:      *boltz.NewExtEntity(entity.Id, entity.Tags),
		Name:               entity.Name,
		TerminatorStrategy: entity.TerminatorStrategy,
	}
}

func newServiceController(controllers *Controllers) *ServiceController {
	result := &ServiceController{
		baseController: newController(controllers, controllers.stores.Service),
		cache:          cmap.New(),
		store:          controllers.stores.Service,
	}
	result.impl = result

	controllers.stores.Terminator.On(boltz.EventCreate, result.terminatorChanged)
	controllers.stores.Terminator.On(boltz.EventUpdate, result.terminatorChanged)
	controllers.stores.Terminator.On(boltz.EventDelete, result.terminatorChanged)

	return result
}

type ServiceController struct {
	baseController
	cache cmap.ConcurrentMap
	store db.ServiceStore
}

func (ctrl *ServiceController) newModelEntity() boltEntitySink {
	return &Service{}
}

func (ctrl *ServiceController) terminatorChanged(params ...interface{}) {
	for _, entity := range params {
		if terminator, ok := entity.(*db.Terminator); ok {
			ctrl.RemoveFromCache(terminator.Service)
		}
	}
}

func (ctrl *ServiceController) Create(s *Service) error {
	err := ctrl.db.Update(func(tx *bbolt.Tx) error {
		ctx := boltz.NewMutateContext(tx)
		if err := ctrl.store.Create(ctx, s.toBolt()); err != nil {
			return err
		}
		for _, terminator := range s.Terminators {
			terminator.Service = s.Id
			if _, err := ctrl.Terminators.createInTx(ctx, terminator); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	ctrl.cache.Set(s.Id, s)
	return nil
}

func (ctrl *ServiceController) Update(s *Service) error {
	err := ctrl.db.Update(func(tx *bbolt.Tx) error {
		return ctrl.store.Update(boltz.NewMutateContext(tx), s.toBolt(), nil)
	})
	if err != nil {
		return err
	}
	ctrl.cache.Set(s.Id, s)
	return nil
}

func (ctrl *ServiceController) Read(id string) (entity *Service, err error) {
	err = ctrl.db.View(func(tx *bbolt.Tx) error {
		entity, err = ctrl.readInTx(tx, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return entity, err
}

func (ctrl *ServiceController) readInTx(tx *bbolt.Tx, id string) (*Service, error) {
	if t, found := ctrl.cache.Get(id); found {
		return t.(*Service), nil
	}

	entity := &Service{}
	if err := ctrl.readEntityInTx(tx, id, entity); err != nil {
		return nil, err
	}

	ctrl.cache.Set(id, entity)
	return entity, nil
}

func (ctrl *ServiceController) Delete(id string) error {
	err := ctrl.db.Update(func(tx *bbolt.Tx) error {
		return ctrl.store.DeleteById(boltz.NewMutateContext(tx), id)
	})
	if err == nil {
		ctrl.cache.Remove(id)
	}
	return err
}

func (ctrl *ServiceController) RemoveFromCache(id string) {
	pfxlog.Logger().Debugf("removed service from cache: %v", id)
	ctrl.cache.Remove(id)
}
