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
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/command"
	"github.com/openziti/ziti/controller/models"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

const annotationsBucketName = "annotations"

type EntityManager[E models.Entity] interface {
	models.EntityRetriever[E]
	command.EntityDeleter
	GetEnv() Env

	newModelEntity() E
	readEntityInTx(tx *bbolt.Tx, id string, modelEntity E) error
}

func newBaseEntityManager[ME edgeEntity[PE], PE boltz.ExtEntity](env Env, store boltz.EntityStore[PE]) baseEntityManager[ME, PE] {
	return baseEntityManager[ME, PE]{
		BaseEntityManager: models.BaseEntityManager[PE]{
			Store: store,
		},
		env: env,
	}
}

type baseEntityManager[ME edgeEntity[PE], PE boltz.ExtEntity] struct {
	models.BaseEntityManager[PE]
	env  Env
	impl EntityManager[ME]
}

func (self *baseEntityManager[ME, PE]) Dispatch(command command.Command) error {
	return self.env.GetManagers().Command.Dispatch(command)
}

func (self *baseEntityManager[ME, PE]) GetEntityTypeId() string {
	// default this to the store entity type and let individual controllers override it where
	// needed to avoid collisions (e.g. edge service/router)
	return self.GetStore().GetEntityType()
}

func (self *baseEntityManager[ME, PE]) GetStore() boltz.EntityStore[PE] {
	return self.Store
}

func (self *baseEntityManager[ME, PE]) GetDb() boltz.Db {
	return self.env.GetDbProvider().GetDb()
}

func (self *baseEntityManager[ME, PE]) GetEnv() Env {
	return self.env
}

func (self *baseEntityManager[ME, PE]) BaseLoad(id string) (ME, error) {
	entity := self.impl.newModelEntity()
	if err := self.readEntity(id, entity); err != nil {
		return *new(ME), err
	}
	return entity, nil
}

func (self *baseEntityManager[ME, PE]) BaseLoadInTx(tx *bbolt.Tx, id string) (ME, error) {
	entity := self.impl.newModelEntity()
	if err := self.readEntityInTx(tx, id, entity); err != nil {
		return *new(ME), err
	}
	return entity, nil
}

func (self *baseEntityManager[ME, PE]) BaseList(query string) (*models.EntityListResult[ME], error) {
	result := &models.EntityListResult[ME]{Loader: self}
	if err := self.ListWithHandler(query, result.Collect); err != nil {
		return nil, err
	}
	return result, nil
}

func (self *baseEntityManager[ME, PE]) BasePreparedList(query ast.Query) (*models.EntityListResult[ME], error) {
	result := &models.EntityListResult[ME]{Loader: self}
	if err := self.PreparedListWithHandler(query, result.Collect); err != nil {
		return nil, err
	}
	return result, nil
}

func (self *baseEntityManager[ME, PE]) BasePreparedListIndexed(cursorProvider ast.SetCursorProvider, query ast.Query) (*models.EntityListResult[ME], error) {
	result := &models.EntityListResult[ME]{Loader: self}
	if err := self.PreparedListIndexed(cursorProvider, query, result.Collect); err != nil {
		return nil, err
	}
	return result, nil
}

func (self *baseEntityManager[ME, PE]) PreparedListWithHandler(query ast.Query, resultHandler models.ListResultHandler) error {
	return self.GetDb().View(func(tx *bbolt.Tx) error {
		return self.PreparedListWithTx(tx, query, resultHandler)
	})
}

func (self *baseEntityManager[ME, PE]) PreparedListIndexed(cursorProvider ast.SetCursorProvider, query ast.Query, resultHandler models.ListResultHandler) error {
	return self.GetDb().View(func(tx *bbolt.Tx) error {
		return self.PreparedListIndexedWithTx(tx, cursorProvider, query, resultHandler)
	})
}

func (self *baseEntityManager[ME, PE]) PreparedListAssociatedWithHandler(id string, association string, query ast.Query, handler models.ListResultHandler) error {
	return self.GetDb().View(func(tx *bbolt.Tx) error {
		return self.PreparedListAssociatedWithTx(tx, id, association, query, handler)
	})
}

func (self *baseEntityManager[ME, PE]) createEntity(modelEntity edgeEntity[PE], ctx boltz.MutateContext) (string, error) {
	var id string
	err := self.GetDb().Update(ctx, func(ctx boltz.MutateContext) error {
		var err error
		id, err = self.createEntityInTx(ctx, modelEntity)
		return err
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

func (self *baseEntityManager[ME, PE]) createEntityInTx(ctx boltz.MutateContext, modelEntity edgeEntity[PE]) (string, error) {
	if modelEntity == nil {
		return "", errors.Errorf("can't create %v with nil value", self.Store.GetEntityType())
	}
	if modelEntity.GetId() == "" {
		modelEntity.SetId(eid.New())
	}

	boltEntity, err := modelEntity.toBoltEntityForCreate(ctx.Tx(), self.env)
	if err != nil {
		return "", err
	}

	if err = self.ValidateNameOnCreate(ctx.Tx(), boltEntity); err != nil {
		return "", err
	}

	if err := self.GetStore().Create(ctx, boltEntity); err != nil {
		pfxlog.Logger().WithError(err).Errorf("could not create %v in bolt storage", self.GetStore().GetSingularEntityType())
		return "", err
	}

	return modelEntity.GetId(), nil
}

func (self *baseEntityManager[ME, PE]) updateEntityBatch(modelEntity edgeEntity[PE], checker boltz.FieldChecker, changeCtx *change.Context) error {
	return self.GetDb().Batch(changeCtx.NewMutateContext(), func(ctx boltz.MutateContext) error {
		existing, found, err := self.GetStore().FindById(ctx.Tx(), modelEntity.GetId())
		if err != nil {
			return err
		}
		if !found {
			return boltz.NewNotFoundError(self.GetStore().GetSingularEntityType(), "id", modelEntity.GetId())
		}
		boltEntity, err := modelEntity.toBoltEntityForUpdate(ctx.Tx(), self.env, checker)
		if err != nil {
			return err
		}

		if err = self.ValidateNameOnUpdate(ctx, boltEntity, existing, checker); err != nil {
			return nil
		}

		if err := self.GetStore().Update(ctx, boltEntity, checker); err != nil {
			pfxlog.Logger().WithError(err).Errorf("batch: could not update %v entity", self.GetStore().GetEntityType())
			return err
		}
		return nil
	})
}

func (self *baseEntityManager[ME, PE]) updateEntity(modelEntity ME, checker boltz.FieldChecker, ctx boltz.MutateContext) error {
	return self.GetDb().Update(ctx, func(ctx boltz.MutateContext) error {
		existing, found, err := self.GetStore().FindById(ctx.Tx(), modelEntity.GetId())
		if err != nil {
			return err
		}
		if !found {
			return boltz.NewNotFoundError(self.GetStore().GetSingularEntityType(), "id", modelEntity.GetId())
		}

		boltEntity, err := modelEntity.toBoltEntityForUpdate(ctx.Tx(), self.env, checker)
		if err != nil {
			return err
		}

		if err = self.ValidateNameOnUpdate(ctx, boltEntity, existing, checker); err != nil {
			return nil
		}

		if err := self.GetStore().Update(ctx, boltEntity, checker); err != nil {
			pfxlog.Logger().WithError(err).Errorf("could not update %v entity", self.GetStore().GetEntityType())
			return err
		}
		return nil
	})
}

func (self *baseEntityManager[ME, PE]) Read(id string) (ME, error) {
	modelEntity := self.impl.newModelEntity()
	err := self.GetDb().View(func(tx *bbolt.Tx) error {
		return self.readEntityInTx(tx, id, modelEntity)
	})
	if err != nil {
		return *new(ME), err
	}
	return modelEntity, nil
}

func (self *baseEntityManager[ME, PE]) readInTx(tx *bbolt.Tx, id string) (ME, error) {
	modelEntity := self.impl.newModelEntity()
	if err := self.readEntityInTx(tx, id, modelEntity); err != nil {
		return *new(ME), err
	}
	return modelEntity, nil
}

func (self *baseEntityManager[ME, PE]) readEntity(id string, modelEntity ME) error {
	return self.GetDb().View(func(tx *bbolt.Tx) error {
		return self.readEntityInTx(tx, id, modelEntity)
	})
}

func (self *baseEntityManager[ME, PE]) readEntityInTx(tx *bbolt.Tx, id string, modelEntity ME) error {
	boltEntity := self.GetStore().GetEntityStrategy().NewEntity()
	found, err := self.GetStore().LoadEntity(tx, id, boltEntity)
	if err != nil {
		return err
	}
	if !found {
		return boltz.NewNotFoundError(self.GetStore().GetSingularEntityType(), "id", id)
	}

	return modelEntity.fillFrom(self.env, tx, boltEntity)
}

func (self *baseEntityManager[ME, PE]) readEntityWithIndex(name string, key []byte, index boltz.ReadIndex, modelEntity ME) error {
	return self.GetDb().View(func(tx *bbolt.Tx) error {
		return self.readEntityInTxWithIndex(name, tx, key, index, modelEntity)
	})
}

func (self *baseEntityManager[ME, PE]) readEntityInTxWithIndex(name string, tx *bbolt.Tx, key []byte, index boltz.ReadIndex, modelEntity ME) error {
	id := index.Read(tx, key)
	if id == nil {
		return boltz.NewNotFoundError(self.GetStore().GetSingularEntityType(), name, string(key))
	}
	return self.readEntityInTx(tx, string(id), modelEntity)
}

func (self *baseEntityManager[ME, PE]) readEntityByQuery(query string) (models.Entity, error) {
	result, err := self.BaseList(query)
	if err != nil {
		return nil, err
	}
	if len(result.GetEntities()) > 0 {
		return result.GetEntities()[0], nil
	}
	return nil, nil
}

func (self *baseEntityManager[ME, PE]) Delete(id string, ctx *change.Context) error {
	cmd := &command.DeleteEntityCommand{
		Context: ctx,
		Deleter: self.impl, // needs to be impl, otherwise we will miss overrides to GetEntityTypeId
		Id:      id,
	}
	return self.Dispatch(cmd)
}

func (self *baseEntityManager[ME, PE]) ApplyDelete(cmd *command.DeleteEntityCommand, ctx boltz.MutateContext) error {
	return self.GetDb().Update(ctx, func(ctx boltz.MutateContext) error {
		return self.Store.DeleteById(ctx, cmd.Id)
	})
}

func (self *baseEntityManager[ME, PE]) deleteEntity(id string, changeCtx *change.Context) error {
	return self.GetDb().Update(changeCtx.NewMutateContext(), func(ctx boltz.MutateContext) error {
		return self.GetStore().DeleteById(ctx, id)
	})
}

func (self *baseEntityManager[ME, PE]) deleteEntityBatch(ids []string, changeCtx *change.Context) error {
	return self.GetDb().Update(changeCtx.NewMutateContext(), func(ctx boltz.MutateContext) error {
		for _, id := range ids {
			if err := self.GetStore().DeleteById(ctx, id); err != nil {
				return err
			}
		}
		return nil
	})
}

func (self *baseEntityManager[ME, PE]) ListWithHandler(queryString string, resultHandler models.ListResultHandler) error {
	return self.GetDb().View(func(tx *bbolt.Tx) error {
		return self.ListWithTx(tx, queryString, resultHandler)
	})
}

func (self *baseEntityManager[ME, PE]) queryRoleAttributes(index boltz.SetReadIndex, queryString string) ([]string, *models.QueryMetaData, error) {
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

func (self *baseEntityManager[ME, PE]) iterateRelatedEntities(id, field string, f func(tx *bbolt.Tx, relatedId string) error) error {
	return self.GetDb().View(func(tx *bbolt.Tx) error {
		return self.iterateRelatedEntitiesInTx(tx, id, field, f)
	})
}

func (self *baseEntityManager[ME, PE]) iterateRelatedEntitiesInTx(tx *bbolt.Tx, id, field string, f func(tx *bbolt.Tx, relatedId string) error) error {
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

func (self *baseEntityManager[ME, PE]) Annotate(ctx boltz.MutateContext, entityId string, key, value string) error {
	entityBucket := self.GetStore().GetEntityBucket(ctx.Tx(), []byte(entityId))
	if entityBucket == nil {
		return boltz.NewNotFoundError(self.GetStore().GetEntityType(), "id", entityId)
	}
	annotationsBucket := entityBucket.GetOrCreatePath(annotationsBucketName)
	annotationsBucket.SetString(key, value, nil)
	return annotationsBucket.GetError()
}

func (self *baseEntityManager[ME, PE]) GetAnnotation(entityId string, key string) (*string, error) {
	var result *string
	err := self.GetDb().View(func(tx *bbolt.Tx) error {
		entityBucket := self.GetStore().GetEntityBucket(tx, []byte(entityId))
		if entityBucket == nil {
			return nil
		}
		if annotationsBucket := entityBucket.GetPath(annotationsBucketName); annotationsBucket != nil {
			result = annotationsBucket.GetString(key)
		}
		return nil
	})
	return result, err
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
