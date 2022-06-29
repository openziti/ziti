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

package network

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/command"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/controller/idgen"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/fabric/ioc"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	"go.etcd.io/bbolt"
)

const (
	CreateDecoder = "CreateDecoder"
	UpdateDecoder = "UpdateDecoder"
	DeleteDecoder = "DeleteDecoder"
)

type Managers struct {
	network     *Network
	db          boltz.Db
	stores      *db.Stores
	Terminators *TerminatorManager
	Routers     *RouterManager
	Services    *ServiceManager
	Inspections *InspectionsManager
	Command     *CommandManager
	Dispatcher  command.Dispatcher
	Registry    ioc.Registry
}

func (self *Managers) getDb() boltz.Db {
	return self.db
}

func (self *Managers) Dispatch(command command.Command) error {
	return self.Dispatcher.Dispatch(command)
}

type creator[T models.Entity] interface {
	command.EntityCreator[T]
	Dispatch(cmd command.Command) error
}

type updater[T models.Entity] interface {
	command.EntityUpdater[T]
	Dispatch(cmd command.Command) error
}

func DispatchCreate[T models.Entity](c creator[T], entity T) error {
	if entity.GetId() == "" {
		id, err := idgen.NewUUIDString()
		if err != nil {
			return err
		}
		entity.SetId(id)
	}

	cmd := &command.CreateEntityCommand[T]{
		Creator: c,
		Entity:  entity,
	}

	return c.Dispatch(cmd)
}

func DispatchUpdate[T models.Entity](u updater[T], entity T, updatedFields boltz.UpdatedFields) error {
	cmd := &command.UpdateEntityCommand[T]{
		Updater:       u,
		Entity:        entity,
		UpdatedFields: updatedFields,
	}

	return u.Dispatch(cmd)
}

type createDecoderF func(entityData []byte) (command.Command, error)

func RegisterCreateDecoder[T models.Entity](managers *Managers, creator command.EntityCreator[T]) {
	entityType := creator.GetEntityTypeId()
	managers.Registry.RegisterSingleton(entityType+CreateDecoder, createDecoderF(func(data []byte) (command.Command, error) {
		entity, err := creator.Unmarshall(data)
		if err != nil {
			return nil, err
		}
		return &command.CreateEntityCommand[T]{
			Entity:  entity,
			Creator: creator,
		}, nil
	}))
}

type updateDecoderF func(entityData []byte, updateFields boltz.UpdatedFields) (command.Command, error)

func RegisterUpdateDecoder[T models.Entity](managers *Managers, updater command.EntityUpdater[T]) {
	entityType := updater.GetEntityTypeId()
	managers.Registry.RegisterSingleton(entityType+UpdateDecoder, updateDecoderF(func(data []byte, updatedFields boltz.UpdatedFields) (command.Command, error) {
		entity, err := updater.Unmarshall(data)
		if err != nil {
			return nil, err
		}
		return &command.UpdateEntityCommand[T]{
			Entity:        entity,
			Updater:       updater,
			UpdatedFields: updatedFields,
		}, nil
	}))
}

type deleteDecoderF func(entityId string) (command.Command, error)

func RegisterDeleteDecoder(managers *Managers, deleter command.EntityDeleter) {
	entityType := deleter.GetEntityTypeId()
	managers.Registry.RegisterSingleton(entityType+UpdateDecoder, deleteDecoderF(func(entityId string) (command.Command, error) {
		return &command.DeleteEntityCommand{
			Deleter: deleter,
			Id:      entityId,
		}, nil
	}))
}

func RegisterManagerDecoder[T models.Entity](managers *Managers, ctrl command.EntityManager[T]) {
	RegisterCreateDecoder[T](managers, ctrl)
	RegisterUpdateDecoder[T](managers, ctrl)
	RegisterDeleteDecoder(managers, ctrl)
}

func NewManagers(network *Network, dispatcher command.Dispatcher, db boltz.Db, stores *db.Stores) *Managers {
	result := &Managers{
		network:    network,
		db:         db,
		stores:     stores,
		Dispatcher: dispatcher,
		Registry:   ioc.NewRegistry(),
	}
	result.Command = newCommandManager(result)
	result.Terminators = newTerminatorManager(result)
	result.Routers = newRouterManager(result)
	result.Services = newServiceManager(result)
	result.Inspections = NewInspectionsManager(network)
	if result.Dispatcher == nil {
		result.Dispatcher = command.LocalDispatcher{}
	}
	result.Command.registerGenericCommands()

	RegisterManagerDecoder[*Service](result, result.Services)
	RegisterManagerDecoder[*Router](result, result.Routers)

	return result
}

type Controller interface {
	models.EntityRetriever
	getManagers() *Managers

	newModelEntity() boltEntitySink
	readEntityInTx(tx *bbolt.Tx, id string, modelEntity boltEntitySink) error
}

type boltEntitySink interface {
	models.Entity
	fillFrom(controller Controller, tx *bbolt.Tx, boltEntity boltz.Entity) error
}

func newBaseEntityManager(managers *Managers, store boltz.CrudStore) baseEntityManager {
	return baseEntityManager{
		BaseEntityManager: models.BaseEntityManager{
			Store: store,
		},
		Managers: managers,
	}
}

type baseEntityManager struct {
	models.BaseEntityManager
	*Managers
	impl Controller
}

func (self *baseEntityManager) GetEntityTypeId() string {
	// default this to the store entity type and let individual managers override it where
	// needed to avoid collisions (e.g. edge service/router)
	return self.GetStore().GetEntityType()
}

func (self *baseEntityManager) Delete(id string) error {
	cmd := &command.DeleteEntityCommand{
		Deleter: self,
		Id:      id,
	}
	return self.Managers.Dispatch(cmd)
}

func (self *baseEntityManager) ApplyDelete(cmd *command.DeleteEntityCommand) error {
	return self.db.Update(func(tx *bbolt.Tx) error {
		ctx := boltz.NewMutateContext(tx)
		return self.Store.DeleteById(ctx, cmd.Id)
	})
}

func (ctrl *baseEntityManager) BaseLoad(id string) (models.Entity, error) {
	entity := ctrl.impl.newModelEntity()
	if err := ctrl.readEntity(id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (ctrl *baseEntityManager) BaseLoadInTx(tx *bbolt.Tx, id string) (models.Entity, error) {
	entity := ctrl.impl.newModelEntity()
	if err := ctrl.readEntityInTx(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (ctrl *baseEntityManager) getManagers() *Managers {
	return ctrl.Managers
}

func (ctrl *baseEntityManager) readEntity(id string, modelEntity boltEntitySink) error {
	return ctrl.db.View(func(tx *bbolt.Tx) error {
		return ctrl.readEntityInTx(tx, id, modelEntity)
	})
}

func (ctrl *baseEntityManager) readEntityInTx(tx *bbolt.Tx, id string, modelEntity boltEntitySink) error {
	boltEntity := ctrl.impl.GetStore().NewStoreEntity()
	found, err := ctrl.impl.GetStore().BaseLoadOneById(tx, id, boltEntity)
	if err != nil {
		return err
	}
	if !found {
		return boltz.NewNotFoundError(ctrl.impl.GetStore().GetSingularEntityType(), "id", id)
	}

	return modelEntity.fillFrom(ctrl.impl, tx, boltEntity)
}

func (ctrl *baseEntityManager) BaseList(query string) (*models.EntityListResult, error) {
	result := &models.EntityListResult{Loader: ctrl}
	err := ctrl.list(query, result.Collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (ctrl *baseEntityManager) list(queryString string, resultHandler models.ListResultHandler) error {
	return ctrl.db.View(func(tx *bbolt.Tx) error {
		return ctrl.ListWithTx(tx, queryString, resultHandler)
	})
}

func (ctrl *baseEntityManager) BasePreparedList(query ast.Query) (*models.EntityListResult, error) {
	result := &models.EntityListResult{Loader: ctrl}
	err := ctrl.preparedList(query, result.Collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (ctrl *baseEntityManager) preparedList(query ast.Query, resultHandler models.ListResultHandler) error {
	return ctrl.db.View(func(tx *bbolt.Tx) error {
		return ctrl.PreparedListWithTx(tx, query, resultHandler)
	})
}

func (ctrl *baseEntityManager) BasePreparedListAssociated(id string, typeLoader models.EntityRetriever, query ast.Query) (*models.EntityListResult, error) {
	result := &models.EntityListResult{Loader: ctrl}
	err := ctrl.db.View(func(tx *bbolt.Tx) error {
		return ctrl.PreparedListAssociatedWithTx(tx, id, typeLoader.GetStore().GetEntityType(), query, result.Collect)
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}

type boltEntitySource interface {
	models.Entity
	toBolt() boltz.Entity
}

func (ctrl *baseEntityManager) updateGeneral(modelEntity boltEntitySource, checker boltz.FieldChecker) error {
	return ctrl.db.Update(func(tx *bbolt.Tx) error {
		ctx := boltz.NewMutateContext(tx)
		existing := ctrl.GetStore().NewStoreEntity()
		found, err := ctrl.GetStore().BaseLoadOneById(tx, modelEntity.GetId(), existing)
		if err != nil {
			return err
		}
		if !found {
			return boltz.NewNotFoundError(ctrl.GetStore().GetSingularEntityType(), "id", modelEntity.GetId())
		}

		boltEntity := modelEntity.toBolt()

		if err := ctrl.ValidateNameOnUpdate(ctx, boltEntity, existing, checker); err != nil {
			return err
		}

		if err := ctrl.GetStore().Update(ctx, boltEntity, checker); err != nil {
			pfxlog.Logger().WithError(err).Errorf("could not update %v entity", ctrl.GetStore().GetEntityType())
			return err
		}
		return nil
	})
}
