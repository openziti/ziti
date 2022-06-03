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
	"github.com/openziti/fabric/controller/command"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/fabric/pb/cmd_pb"
	"github.com/openziti/storage/boltz"
	"github.com/orcaman/concurrent-map/v2"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"google.golang.org/protobuf/proto"
	"reflect"
)

type Service struct {
	models.BaseEntity
	Name               string
	TerminatorStrategy string
	Terminators        []*Terminator
}

func (self *Service) GetName() string {
	return self.Name
}

func (entity *Service) fillFrom(ctrl Controller, tx *bbolt.Tx, boltEntity boltz.Entity) error {
	boltService, ok := boltEntity.(*db.Service)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model service", reflect.TypeOf(boltEntity))
	}
	entity.Name = boltService.Name
	entity.TerminatorStrategy = boltService.TerminatorStrategy
	entity.FillCommon(boltService)

	terminatorIds := ctrl.getManagers().stores.Service.GetRelatedEntitiesIdList(tx, entity.Id, db.EntityTypeTerminators)
	for _, terminatorId := range terminatorIds {
		if terminator, _ := ctrl.getManagers().Terminators.readInTx(tx, terminatorId); terminator != nil {
			entity.Terminators = append(entity.Terminators, terminator)
		}
	}

	return nil
}

func (entity *Service) toBolt() boltz.Entity {
	return &db.Service{
		BaseExtEntity:      *boltz.NewExtEntity(entity.Id, entity.Tags),
		Name:               entity.Name,
		TerminatorStrategy: entity.TerminatorStrategy,
	}
}

func newServiceManager(managers *Managers) *ServiceManager {
	result := &ServiceManager{
		baseEntityManager: newBaseEntityManager(managers, managers.stores.Service),
		cache:             cmap.New[*Service](),
		store:             managers.stores.Service,
	}
	result.impl = result

	cacheInvalidationF := func(i ...interface{}) {
		for _, val := range i {
			if service, ok := val.(*db.Service); ok {
				result.RemoveFromCache(service.Id)
			} else {
				pfxlog.Logger().Errorf("error in service listener. expected *db.Service, got %T", val)
			}
		}
	}

	managers.stores.Service.AddListener(boltz.EventUpdate, cacheInvalidationF)
	managers.stores.Service.AddListener(boltz.EventDelete, cacheInvalidationF)

	return result
}

type ServiceManager struct {
	baseEntityManager
	cache cmap.ConcurrentMap[*Service]
	store db.ServiceStore
}

func (self *ServiceManager) newModelEntity() boltEntitySink {
	return &Service{}
}

func (self *ServiceManager) NotifyTerminatorChanged(terminator *db.Terminator) *db.Terminator {
	// patched entities may not have all fields, if service is blank, load terminator
	serviceId := terminator.Service
	if serviceId == "" {
		err := self.db.View(func(tx *bbolt.Tx) error {
			t, err := self.stores.Terminator.LoadOneById(tx, terminator.Id)
			if t != nil {
				terminator = t
			}
			return err
		})
		if err != nil {
			self.clearCache()
			return terminator
		}
		serviceId = terminator.Service
	}
	pfxlog.Logger().Debugf("clearing service from cache: %v", serviceId)
	self.RemoveFromCache(serviceId)
	return terminator
}

func (self *ServiceManager) Create(entity *Service) error {
	return DispatchCreate[*Service](self, entity)
}

func (self *ServiceManager) ApplyCreate(cmd *command.CreateEntityCommand[*Service]) error {
	s := cmd.Entity
	err := self.db.Update(func(tx *bbolt.Tx) error {
		ctx := boltz.NewMutateContext(tx)
		if err := self.ValidateNameOnCreate(ctx, s); err != nil {
			return err
		}
		if err := self.store.Create(ctx, s.toBolt()); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	// don't cache, wait for first read. entity may not match data store as data store may have set defaults
	return nil
}

func (self *ServiceManager) Update(entity *Service, updatedFields boltz.UpdatedFields) error {
	return DispatchUpdate[*Service](self, entity, updatedFields)
}

func (self *ServiceManager) ApplyUpdate(cmd *command.UpdateEntityCommand[*Service]) error {
	if err := self.updateGeneral(cmd.Entity, cmd.UpdatedFields); err != nil {
		return err
	}
	self.RemoveFromCache(cmd.Entity.Id)
	return nil
}

func (self *ServiceManager) Read(id string) (entity *Service, err error) {
	err = self.db.View(func(tx *bbolt.Tx) error {
		entity, err = self.readInTx(tx, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return entity, err
}

func (self *ServiceManager) GetIdForName(id string) (string, error) {
	var result []byte
	err := self.db.View(func(tx *bbolt.Tx) error {
		result = self.store.GetNameIndex().Read(tx, []byte(id))
		return nil
	})
	return string(result), err
}

func (self *ServiceManager) readInTx(tx *bbolt.Tx, id string) (*Service, error) {
	if service, found := self.cache.Get(id); found {
		return service, nil
	}

	entity := &Service{}
	if err := self.readEntityInTx(tx, id, entity); err != nil {
		return nil, err
	}

	self.cacheService(entity)
	return entity, nil
}

func (self *ServiceManager) cacheService(service *Service) {
	pfxlog.Logger().Tracef("updated service cache: %v", service.Id)
	self.cache.Set(service.Id, service)
}

func (self *ServiceManager) RemoveFromCache(id string) {
	pfxlog.Logger().Debugf("removed service from cache: %v", id)
	self.cache.Remove(id)
}

func (self *ServiceManager) clearCache() {
	pfxlog.Logger().Debugf("clearing all services from cache")
	for _, key := range self.cache.Keys() {
		self.cache.Remove(key)
	}
}

func (self *ServiceManager) Marshall(entity *Service) ([]byte, error) {
	tags, err := cmd_pb.EncodeTags(entity.Tags)
	if err != nil {
		return nil, err
	}

	msg := &cmd_pb.Service{
		Id:                 entity.Id,
		Name:               entity.Name,
		TerminatorStrategy: entity.TerminatorStrategy,
		Tags:               tags,
	}

	return proto.Marshal(msg)
}

func (self *ServiceManager) Unmarshall(bytes []byte) (*Service, error) {
	msg := &cmd_pb.Service{}
	if err := proto.Unmarshal(bytes, msg); err != nil {
		return nil, err
	}

	return &Service{
		BaseEntity: models.BaseEntity{
			Id:   msg.Id,
			Tags: cmd_pb.DecodeTags(msg.Tags),
		},
		Name:               msg.Name,
		TerminatorStrategy: msg.TerminatorStrategy,
	}, nil
}
