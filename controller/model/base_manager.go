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

package model

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/eid"
	"github.com/openziti/fabric/controller/command"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
)

type EntityManager interface {
	models.EntityRetriever[models.Entity]
	command.EntityDeleter
	GetEnv() Env

	newModelEntity() edgeEntity
	readEntityInTx(tx *bbolt.Tx, id string, modelEntity edgeEntity) error
}

func newBaseEntityManager(env Env, store boltz.CrudStore) baseEntityManager {
	return baseEntityManager{
		BaseEntityManager: models.BaseEntityManager{
			Store: store,
		},
		env: env,
	}
}

type baseEntityManager struct {
	models.BaseEntityManager
	env  Env
	impl EntityManager
}

func (self *baseEntityManager) Dispatch(command command.Command) error {
	return self.env.GetManagers().Command.Dispatch(command)
}

func (self *baseEntityManager) GetEntityTypeId() string {
	// default this to the store entity type and let individual controllers override it where
	// needed to avoid collisions (e.g. edge service/router)
	return self.GetStore().GetEntityType()
}

func (self *baseEntityManager) GetStore() boltz.CrudStore {
	return self.Store
}

func (self *baseEntityManager) GetDb() boltz.Db {
	return self.env.GetDbProvider().GetDb()
}

func (self *baseEntityManager) GetEnv() Env {
	return self.env
}

func (self *baseEntityManager) BaseLoad(id string) (models.Entity, error) {
	entity := self.impl.newModelEntity()
	if err := self.readEntity(id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (self *baseEntityManager) BaseLoadInTx(tx *bbolt.Tx, id string) (models.Entity, error) {
	entity := self.impl.newModelEntity()
	if err := self.readEntityInTx(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (self *baseEntityManager) BaseList(query string) (*models.EntityListResult[models.Entity], error) {
	result := &models.EntityListResult[models.Entity]{Loader: self}
	if err := self.ListWithHandler(query, result.Collect); err != nil {
		return nil, err
	}
	return result, nil
}

func (self *baseEntityManager) BasePreparedList(query ast.Query) (*models.EntityListResult[models.Entity], error) {
	result := &models.EntityListResult[models.Entity]{Loader: self}
	if err := self.PreparedListWithHandler(query, result.Collect); err != nil {
		return nil, err
	}
	return result, nil
}

func (self *baseEntityManager) BasePreparedListIndexed(cursorProvider ast.SetCursorProvider, query ast.Query) (*models.EntityListResult[models.Entity], error) {
	result := &models.EntityListResult[models.Entity]{Loader: self}
	if err := self.preparedListIndexed(cursorProvider, query, result.Collect); err != nil {
		return nil, err
	}
	return result, nil
}

func (self *baseEntityManager) PreparedListWithHandler(query ast.Query, resultHandler models.ListResultHandler) error {
	return self.GetDb().View(func(tx *bbolt.Tx) error {
		return self.PreparedListWithTx(tx, query, resultHandler)
	})
}

func (self *baseEntityManager) preparedListIndexed(cursorProvider ast.SetCursorProvider, query ast.Query, resultHandler models.ListResultHandler) error {
	return self.GetDb().View(func(tx *bbolt.Tx) error {
		return self.PreparedListIndexedWithTx(tx, cursorProvider, query, resultHandler)
	})
}

func (self *baseEntityManager) PreparedListAssociatedWithHandler(id string, association string, query ast.Query, handler models.ListResultHandler) error {
	return self.GetDb().View(func(tx *bbolt.Tx) error {
		return self.PreparedListAssociatedWithTx(tx, id, association, query, handler)
	})
}

func (self *baseEntityManager) createEntity(modelEntity edgeEntity) (string, error) {
	var id string
	err := self.GetDb().Update(func(tx *bbolt.Tx) error {
		var err error
		id, err = self.createEntityInTx(boltz.NewMutateContext(tx), modelEntity)
		return err
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

func (self *baseEntityManager) createEntityInTx(ctx boltz.MutateContext, modelEntity edgeEntity) (string, error) {
	if modelEntity == nil {
		return "", errors.Errorf("can't create %v with nil value", self.Store.GetEntityType())
	}
	if modelEntity.GetId() == "" {
		modelEntity.SetId(eid.New())
	}

	boltEntity, err := modelEntity.toBoltEntityForCreate(ctx.Tx(), self.impl)
	if err != nil {
		return "", err
	}

	if err = self.ValidateNameOnCreate(ctx, boltEntity); err != nil {
		return "", err
	}

	if err := self.GetStore().Create(ctx, boltEntity); err != nil {
		pfxlog.Logger().WithError(err).Errorf("could not create %v in bolt storage", self.GetStore().GetSingularEntityType())
		return "", err
	}

	return modelEntity.GetId(), nil
}

func (self *baseEntityManager) updateEntityBatch(modelEntity edgeEntity, checker boltz.FieldChecker) error {
	return self.GetDb().Batch(func(tx *bbolt.Tx) error {
		ctx := boltz.NewMutateContext(tx)
		existing := self.GetStore().NewStoreEntity()
		found, err := self.GetStore().BaseLoadOneById(tx, modelEntity.GetId(), existing)
		if err != nil {
			return err
		}
		if !found {
			return boltz.NewNotFoundError(self.GetStore().GetSingularEntityType(), "id", modelEntity.GetId())
		}
		boltEntity, err := modelEntity.toBoltEntityForUpdate(tx, self.impl, checker)
		if err != nil {
			return err
		}

		// validate name for named entities
		if namedEntity, ok := boltEntity.(boltz.NamedExtEntity); ok {
			existingNamed := existing.(boltz.NamedExtEntity)
			if (checker == nil || checker.IsUpdated("name")) && namedEntity.GetName() != existingNamed.GetName() {
				if namedEntity.GetName() == "" {
					return errorz.NewFieldError("name is required", "name", namedEntity.GetName())
				}
				if nameIndexStore, ok := self.GetStore().(persistence.NameIndexedStore); ok {
					if nameIndexStore.GetNameIndex().Read(ctx.Tx(), []byte(namedEntity.GetName())) != nil {
						return errorz.NewFieldError("name is must be unique", "name", namedEntity.GetName())
					}
				} else {
					pfxlog.Logger().Errorf("batch: entity of type %v is named, but store doesn't have name index", reflect.TypeOf(boltEntity))
				}
			}
		}

		if err := self.GetStore().Update(ctx, boltEntity, checker); err != nil {
			pfxlog.Logger().WithError(err).Errorf("batch: could not update %v entity", self.GetStore().GetEntityType())
			return err
		}
		return nil
	})
}

func (self *baseEntityManager) updateEntity(modelEntity edgeEntity, checker boltz.FieldChecker) error {
	return self.GetDb().Update(func(tx *bbolt.Tx) error {
		ctx := boltz.NewMutateContext(tx)
		existing := self.GetStore().NewStoreEntity()
		found, err := self.GetStore().BaseLoadOneById(tx, modelEntity.GetId(), existing)
		if err != nil {
			return err
		}
		if !found {
			return boltz.NewNotFoundError(self.GetStore().GetSingularEntityType(), "id", modelEntity.GetId())
		}
		boltEntity, err := modelEntity.toBoltEntityForUpdate(tx, self.impl, checker)
		if err != nil {
			return err
		}

		// validate name for named entities
		if namedEntity, ok := boltEntity.(boltz.NamedExtEntity); ok {
			existingNamed := existing.(boltz.NamedExtEntity)
			if (checker == nil || checker.IsUpdated("name")) && namedEntity.GetName() != existingNamed.GetName() {
				if namedEntity.GetName() == "" {
					return errorz.NewFieldError("name is required", "name", namedEntity.GetName())
				}
				if nameIndexStore, ok := self.GetStore().(persistence.NameIndexedStore); ok {
					if nameIndexStore.GetNameIndex().Read(ctx.Tx(), []byte(namedEntity.GetName())) != nil {
						return errorz.NewFieldError("name is must be unique", "name", namedEntity.GetName())
					}
				} else {
					pfxlog.Logger().Errorf("entity of type %v is named, but store doesn't have name index", reflect.TypeOf(boltEntity))
				}
			}
		}

		if err := self.GetStore().Update(ctx, boltEntity, checker); err != nil {
			pfxlog.Logger().WithError(err).Errorf("could not update %v entity", self.GetStore().GetEntityType())
			return err
		}
		return nil
	})
}

func (self *baseEntityManager) readEntity(id string, modelEntity edgeEntity) error {
	return self.GetDb().View(func(tx *bbolt.Tx) error {
		return self.readEntityInTx(tx, id, modelEntity)
	})
}

func (self *baseEntityManager) readEntityInTx(tx *bbolt.Tx, id string, modelEntity edgeEntity) error {
	boltEntity := self.GetStore().NewStoreEntity()
	found, err := self.GetStore().BaseLoadOneById(tx, id, boltEntity)
	if err != nil {
		return err
	}
	if !found {
		return boltz.NewNotFoundError(self.GetStore().GetSingularEntityType(), "id", id)
	}

	return modelEntity.fillFrom(self.impl, tx, boltEntity)
}

func (self *baseEntityManager) readEntityWithIndex(name string, key []byte, index boltz.ReadIndex, modelEntity edgeEntity) error {
	return self.GetDb().View(func(tx *bbolt.Tx) error {
		return self.readEntityInTxWithIndex(name, tx, key, index, modelEntity)
	})
}

func (self *baseEntityManager) readEntityInTxWithIndex(name string, tx *bbolt.Tx, key []byte, index boltz.ReadIndex, modelEntity edgeEntity) error {
	id := index.Read(tx, key)
	if id == nil {
		return boltz.NewNotFoundError(self.GetStore().GetSingularEntityType(), name, string(key))
	}
	return self.readEntityInTx(tx, string(id), modelEntity)
}

func (self *baseEntityManager) readEntityByQuery(query string) (models.Entity, error) {
	result, err := self.BaseList(query)
	if err != nil {
		return nil, err
	}
	if len(result.GetEntities()) > 0 {
		return result.GetEntities()[0], nil
	}
	return nil, nil
}

func (self *baseEntityManager) Delete(id string) error {
	cmd := &command.DeleteEntityCommand{
		Deleter: self.impl, // needs to be impl, otherwise we will miss overrides to GetEntityTypeId
		Id:      id,
	}
	return self.Dispatch(cmd)
}

func (self *baseEntityManager) ApplyDelete(cmd *command.DeleteEntityCommand) error {
	return self.GetDb().Update(func(tx *bbolt.Tx) error {
		ctx := boltz.NewMutateContext(tx)
		return self.Store.DeleteById(ctx, cmd.Id)
	})
}

func (self *baseEntityManager) deleteEntity(id string) error {
	return self.GetDb().Update(func(tx *bbolt.Tx) error {
		return self.GetStore().DeleteById(boltz.NewMutateContext(tx), id)
	})
}

func (self *baseEntityManager) deleteEntityBatch(ids []string) error {
	return self.GetDb().Update(func(tx *bbolt.Tx) error {
		for _, id := range ids {
			if err := self.GetStore().DeleteById(boltz.NewMutateContext(tx), id); err != nil {
				return err
			}
		}
		return nil
	})
}

func (self *baseEntityManager) ListWithHandler(queryString string, resultHandler models.ListResultHandler) error {
	return self.GetDb().View(func(tx *bbolt.Tx) error {
		return self.ListWithTx(tx, queryString, resultHandler)
	})
}

func (self *baseEntityManager) queryRoleAttributes(index boltz.SetReadIndex, queryString string) ([]string, *models.QueryMetaData, error) {
	query, err := ast.Parse(self.env.GetStores().Index, queryString)
	if err != nil {
		return nil, nil, err
	}

	var results []string
	var count int64
	err = self.GetDb().View(func(tx *bbolt.Tx) error {
		results, count, err = self.env.GetStores().Index.QueryWithCursorC(tx, index.OpenKeyCursor, query)
		return err
	})

	if err != nil {
		return nil, nil, err
	}

	qmd := &models.QueryMetaData{
		Count:            count,
		Limit:            *query.GetLimit(),
		Offset:           *query.GetSkip(),
		FilterableFields: self.env.GetStores().Index.GetPublicSymbols(),
	}

	return results, qmd, nil
}

func (self *baseEntityManager) iterateRelatedEntities(id, field string, f func(tx *bbolt.Tx, relatedId string) error) error {
	return self.GetDb().View(func(tx *bbolt.Tx) error {
		return self.iterateRelatedEntitiesInTx(tx, id, field, f)
	})
}

func (self *baseEntityManager) iterateRelatedEntitiesInTx(tx *bbolt.Tx, id, field string, f func(tx *bbolt.Tx, relatedId string) error) error {
	cursor := self.Store.GetRelatedEntitiesCursor(tx, id, field, true)
	for cursor.IsValid() {
		key := cursor.Current()
		if err := f(tx, string(key)); err != nil {
			return err
		}
		cursor.Next()
	}
	return nil
}

type AndFieldChecker struct {
	first  boltz.FieldChecker
	second boltz.FieldChecker
}

func (checker *AndFieldChecker) IsUpdated(field string) bool {
	return checker.first.IsUpdated(field) && checker.second.IsUpdated(field)
}

type OrFieldChecker struct {
	first  boltz.FieldChecker
	second boltz.FieldChecker
}

func NewOrFieldChecker(checker boltz.FieldChecker, fields ...string) *OrFieldChecker {
	return &OrFieldChecker{first: NewFieldChecker(fields...), second: checker}
}

func (checker *OrFieldChecker) IsUpdated(field string) bool {
	return checker.first.IsUpdated(field) || checker.second.IsUpdated(field)
}

func NewFieldChecker(fields ...string) boltz.FieldChecker {
	result := boltz.MapFieldChecker{}
	for _, field := range fields {
		result[field] = struct{}{}
	}
	return result
}
