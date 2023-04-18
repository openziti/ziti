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
	"github.com/openziti/fabric/controller/change"
	"github.com/openziti/fabric/controller/command"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/controller/fields"
	"github.com/openziti/fabric/controller/idgen"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/fabric/ioc"
	"github.com/openziti/fabric/pb/cmd_pb"
	"github.com/openziti/foundation/v2/versions"
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

func DispatchCreate[T models.Entity](c creator[T], entity T, ctx *change.Context) error {
	if entity.GetId() == "" {
		id, err := idgen.NewUUIDString()
		if err != nil {
			return err
		}
		entity.SetId(id)
	}

	cmd := &command.CreateEntityCommand[T]{
		Context: ctx,
		Creator: c,
		Entity:  entity,
	}

	return c.Dispatch(cmd)
}

func DispatchUpdate[T models.Entity](u updater[T], entity T, updatedFields fields.UpdatedFields, ctx *change.Context) error {
	cmd := &command.UpdateEntityCommand[T]{
		Context:       ctx,
		Updater:       u,
		Entity:        entity,
		UpdatedFields: updatedFields,
	}

	return u.Dispatch(cmd)
}

type createDecoderF func(cmd *cmd_pb.CreateEntityCommand) (command.Command, error)

func RegisterCreateDecoder[T models.Entity](managers *Managers, creator command.EntityCreator[T]) {
	entityType := creator.GetEntityTypeId()
	managers.Registry.RegisterSingleton(entityType+CreateDecoder, createDecoderF(func(cmd *cmd_pb.CreateEntityCommand) (command.Command, error) {
		entity, err := creator.Unmarshall(cmd.EntityData)
		if err != nil {
			return nil, err
		}
		return &command.CreateEntityCommand[T]{
			Context: change.FromProtoBuf(cmd.Ctx),
			Entity:  entity,
			Creator: creator,
			Flags:   cmd.Flags,
		}, nil
	}))
}

type updateDecoderF func(cmd *cmd_pb.UpdateEntityCommand) (command.Command, error)

func RegisterUpdateDecoder[T models.Entity](managers *Managers, updater command.EntityUpdater[T]) {
	entityType := updater.GetEntityTypeId()
	managers.Registry.RegisterSingleton(entityType+UpdateDecoder, updateDecoderF(func(cmd *cmd_pb.UpdateEntityCommand) (command.Command, error) {
		entity, err := updater.Unmarshall(cmd.EntityData)
		if err != nil {
			return nil, err
		}
		return &command.UpdateEntityCommand[T]{
			Context:       change.FromProtoBuf(cmd.Ctx),
			Entity:        entity,
			Updater:       updater,
			UpdatedFields: fields.SliceToUpdatedFields(cmd.UpdatedFields),
			Flags:         cmd.Flags,
		}, nil
	}))
}

type deleteDecoderF func(cmd *cmd_pb.DeleteEntityCommand) (command.Command, error)

func RegisterDeleteDecoder(managers *Managers, deleter command.EntityDeleter) {
	entityType := deleter.GetEntityTypeId()
	managers.Registry.RegisterSingleton(entityType+DeleteDecoder, deleteDecoderF(func(cmd *cmd_pb.DeleteEntityCommand) (command.Command, error) {
		return &command.DeleteEntityCommand{
			Context: change.FromProtoBuf(cmd.Ctx),
			Deleter: deleter,
			Id:      cmd.EntityId,
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
		devVersion := versions.MustParseSemVer("0.0.0")
		version := versions.MustParseSemVer(network.VersionProvider.Version())
		result.Dispatcher = &command.LocalDispatcher{
			EncodeDecodeCommands: devVersion.Equals(version),
		}
	}
	result.Command.registerGenericCommands()

	RegisterManagerDecoder[*Service](result, result.Services)
	RegisterManagerDecoder[*Router](result, result.Routers)
	RegisterManagerDecoder[*Terminator](result, result.Terminators)
	RegisterCommand(result, &DeleteTerminatorsBatchCommand{}, &cmd_pb.DeleteTerminatorsBatchCommand{})

	return result
}

type Controller[T models.Entity] interface {
	models.EntityRetriever[T]
	getManagers() *Managers
}

func newBaseEntityManager[ME models.Entity, PE boltz.ExtEntity](managers *Managers, store boltz.EntityStore[PE], newModelEntity func() ME) baseEntityManager[ME, PE] {
	return baseEntityManager[ME, PE]{
		BaseEntityManager: models.BaseEntityManager[PE]{
			Store: store,
		},
		Managers:       managers,
		newModelEntity: newModelEntity,
	}
}

type baseEntityManager[T models.Entity, PE boltz.ExtEntity] struct {
	models.BaseEntityManager[PE]
	*Managers
	newModelEntity func() T
	populateEntity func(entity T, tx *bbolt.Tx, boltEntity boltz.Entity) error
}

func (self *baseEntityManager[ME, PE]) GetEntityTypeId() string {
	// default this to the store entity type and let individual managers override it where
	// needed to avoid collisions (e.g. edge service/router)
	return self.GetStore().GetEntityType()
}

func (self *baseEntityManager[ME, PE]) Delete(id string, ctx *change.Context) error {
	cmd := &command.DeleteEntityCommand{
		Context: ctx,
		Deleter: self,
		Id:      id,
	}
	return self.Managers.Dispatch(cmd)
}

func (self *baseEntityManager[ME, PE]) ApplyDelete(cmd *command.DeleteEntityCommand, ctx boltz.MutateContext) error {
	return self.db.Update(ctx, func(mutateCtx boltz.MutateContext) error {
		return self.Store.DeleteById(ctx, cmd.Id)
	})
}

func (ctrl *baseEntityManager[ME, PE]) BaseLoad(id string) (ME, error) {
	entity := ctrl.newModelEntity()
	if err := ctrl.readEntity(id, entity); err != nil {
		return *new(ME), err
	}
	return entity, nil
}

func (ctrl *baseEntityManager[ME, PE]) BaseLoadInTx(tx *bbolt.Tx, id string) (ME, error) {
	entity := ctrl.newModelEntity()
	if err := ctrl.readEntityInTx(tx, id, entity); err != nil {
		return *new(ME), err
	}
	return entity, nil
}

func (ctrl *baseEntityManager[ME, PE]) readEntity(id string, modelEntity ME) error {
	return ctrl.db.View(func(tx *bbolt.Tx) error {
		return ctrl.readEntityInTx(tx, id, modelEntity)
	})
}

func (ctrl *baseEntityManager[ME, PE]) readEntityInTx(tx *bbolt.Tx, id string, modelEntity ME) error {
	boltEntity, found, err := ctrl.GetStore().FindById(tx, id)
	if err != nil {
		return err
	}
	if !found {
		return boltz.NewNotFoundError(ctrl.GetStore().GetSingularEntityType(), "id", id)
	}

	return ctrl.populateEntity(modelEntity, tx, boltEntity)
}

func (ctrl *baseEntityManager[ME, PE]) BaseList(query string) (*models.EntityListResult[ME], error) {
	result := &models.EntityListResult[ME]{Loader: ctrl}
	err := ctrl.ListWithHandler(query, result.Collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (ctrl *baseEntityManager[ME, PE]) ListWithHandler(queryString string, resultHandler models.ListResultHandler) error {
	return ctrl.db.View(func(tx *bbolt.Tx) error {
		return ctrl.ListWithTx(tx, queryString, resultHandler)
	})
}

func (ctrl *baseEntityManager[ME, PE]) BasePreparedList(query ast.Query) (*models.EntityListResult[ME], error) {
	result := &models.EntityListResult[ME]{Loader: ctrl}
	err := ctrl.PreparedListWithHandler(query, result.Collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (ctrl *baseEntityManager[ME, PE]) PreparedListWithHandler(query ast.Query, resultHandler models.ListResultHandler) error {
	return ctrl.db.View(func(tx *bbolt.Tx) error {
		return ctrl.PreparedListWithTx(tx, query, resultHandler)
	})
}

func (ctrl *baseEntityManager[ME, PE]) PreparedListAssociatedWithHandler(id string, association string, query ast.Query, handler models.ListResultHandler) error {
	return ctrl.db.View(func(tx *bbolt.Tx) error {
		return ctrl.PreparedListAssociatedWithTx(tx, id, association, query, handler)
	})
}

type boltEntitySource[PE boltz.ExtEntity] interface {
	models.Entity
	toBolt() PE
}

func (ctrl *baseEntityManager[ME, PE]) updateGeneral(ctx boltz.MutateContext, modelEntity boltEntitySource[PE], checker boltz.FieldChecker) error {
	return ctrl.db.Update(ctx, func(ctx boltz.MutateContext) error {
		existing, found, err := ctrl.GetStore().FindById(ctx.Tx(), modelEntity.GetId())
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
