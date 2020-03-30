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
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/persistence"
	"github.com/netfoundry/ziti-edge/controller/validation"
	"github.com/netfoundry/ziti-fabric/controller/controllers"
	"github.com/netfoundry/ziti-fabric/controller/models"
	"github.com/netfoundry/ziti-foundation/storage/ast"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
)

type Handler interface {
	models.EntityRetriever

	GetEnv() Env

	newModelEntity() boltEntitySink
	readEntityInTx(tx *bbolt.Tx, id string, modelEntity boltEntitySink) error
}

func newBaseHandler(env Env, store boltz.CrudStore) baseHandler {
	return baseHandler{
		BaseController: models.BaseController{
			Store: store,
		},
		env: env,
	}
}

type baseHandler struct {
	models.BaseController
	env  Env
	impl Handler
}

func (handler *baseHandler) GetStore() boltz.CrudStore {
	return handler.Store
}

func (handler *baseHandler) GetDb() boltz.Db {
	return handler.env.GetDbProvider().GetDb()
}

func (handler *baseHandler) GetEnv() Env {
	return handler.env
}

func (handler *baseHandler) BaseLoad(id string) (models.Entity, error) {
	entity := handler.impl.newModelEntity()
	if err := handler.readEntity(id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (handler *baseHandler) BaseLoadInTx(tx *bbolt.Tx, id string) (models.Entity, error) {
	entity := handler.impl.newModelEntity()
	if err := handler.readEntityInTx(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (handler *baseHandler) BaseList(query string) (*models.EntityListResult, error) {
	result := &models.EntityListResult{Loader: handler}
	if err := handler.list(query, result.Collect); err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *baseHandler) BasePreparedList(query ast.Query) (*models.EntityListResult, error) {
	result := &models.EntityListResult{Loader: handler}
	if err := handler.preparedList(query, result.Collect); err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *baseHandler) BasePreparedListIndexed(cursorProvider ast.SetCursorProvider, query ast.Query) (*models.EntityListResult, error) {
	result := &models.EntityListResult{Loader: handler}
	if err := handler.preparedListIndexed(cursorProvider, query, result.Collect); err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *baseHandler) preparedList(query ast.Query, resultHandler models.ListResultHandler) error {
	return handler.GetDb().View(func(tx *bbolt.Tx) error {
		return handler.PreparedListWithTx(tx, query, resultHandler)
	})
}

func (handler *baseHandler) preparedListIndexed(cursorProvider ast.SetCursorProvider, query ast.Query, resultHandler models.ListResultHandler) error {
	return handler.GetDb().View(func(tx *bbolt.Tx) error {
		return handler.PreparedListIndexedWithTx(tx, cursorProvider, query, resultHandler)
	})
}

func (handler *baseHandler) BasePreparedListAssociated(id string, typeLoader models.EntityRetriever, query ast.Query) (*models.EntityListResult, error) {
	result := &models.EntityListResult{Loader: typeLoader}
	err := handler.GetDb().View(func(tx *bbolt.Tx) error {
		return handler.PreparedListAssociatedWithTx(tx, id, typeLoader.GetStore().GetEntityType(), query, result.Collect)
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *baseHandler) createEntity(modelEntity boltEntitySource) (string, error) {
	var id string
	err := handler.GetDb().Update(func(tx *bbolt.Tx) error {
		var err error
		id, err = handler.createEntityInTx(boltz.NewMutateContext(tx), modelEntity)
		return err
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

func (handler *baseHandler) createEntityInTx(ctx boltz.MutateContext, modelEntity boltEntitySource) (string, error) {
	if modelEntity == nil {
		return "", errors.Errorf("can't create %v with nil value", handler.Store.GetEntityType())
	}
	if modelEntity.GetId() == "" {
		modelEntity.SetId(uuid.New().String())
	}

	boltEntity, err := modelEntity.toBoltEntityForCreate(ctx.Tx(), handler.impl)
	if err != nil {
		return "", err
	}
	store := handler.GetStore()

	// validate name for named entities
	if namedEntity, ok := boltEntity.(boltz.NamedExtEntity); ok {
		if namedEntity.GetName() == "" {
			return "", validation.NewFieldError("name is required", "name", namedEntity.GetName())
		}
		if nameIndexStore, ok := store.(persistence.NameIndexedStore); ok {
			if nameIndexStore.GetNameIndex().Read(ctx.Tx(), []byte(namedEntity.GetName())) != nil {
				return "", validation.NewFieldError("name is must be unique", "name", namedEntity.GetName())
			}
		} else {
			pfxlog.Logger().Errorf("entity of type %v is named, but store doesn't have name index", reflect.TypeOf(boltEntity))
		}
	}

	if err := store.Create(ctx, boltEntity); err != nil {
		pfxlog.Logger().WithError(err).Errorf("could not create %v in bolt storage", handler.GetStore().GetSingularEntityType())
		return "", err
	}

	return modelEntity.GetId(), nil
}

func (handler *baseHandler) updateEntity(modelEntity boltEntitySource, checker boltz.FieldChecker) error {
	return handler.updateGeneral(modelEntity, checker, false)
}

func (handler *baseHandler) patchEntity(modelEntity boltEntitySource, checker boltz.FieldChecker) error {
	return handler.updateGeneral(modelEntity, checker, true)
}

func (handler *baseHandler) updateGeneral(modelEntity boltEntitySource, checker boltz.FieldChecker, patch bool) error {
	return handler.GetDb().Update(func(tx *bbolt.Tx) error {
		ctx := boltz.NewMutateContext(tx)
		existing := handler.GetStore().NewStoreEntity()
		found, err := handler.GetStore().BaseLoadOneById(tx, modelEntity.GetId(), existing)
		if err != nil {
			return err
		}
		if !found {
			return boltz.NewNotFoundError(handler.GetStore().GetSingularEntityType(), "id", modelEntity.GetId())
		}
		var boltEntity boltz.Entity
		if patch {
			boltEntity, err = modelEntity.toBoltEntityForPatch(tx, handler.impl)
		} else {
			boltEntity, err = modelEntity.toBoltEntityForUpdate(tx, handler.impl)
		}
		if err != nil {
			return err
		}

		// validate name for named entities
		if namedEntity, ok := boltEntity.(boltz.NamedExtEntity); ok {
			existingNamed := existing.(boltz.NamedExtEntity)
			if (checker == nil || checker.IsUpdated("name")) && namedEntity.GetName() != existingNamed.GetName() {
				if namedEntity.GetName() == "" {
					return validation.NewFieldError("name is required", "name", namedEntity.GetName())
				}
				if nameIndexStore, ok := handler.GetStore().(persistence.NameIndexedStore); ok {
					if nameIndexStore.GetNameIndex().Read(ctx.Tx(), []byte(namedEntity.GetName())) != nil {
						return validation.NewFieldError("name is must be unique", "name", namedEntity.GetName())
					}
				} else {
					pfxlog.Logger().Errorf("entity of type %v is named, but store doesn't have name index", reflect.TypeOf(boltEntity))
				}
			}
		}

		if err := handler.GetStore().Update(ctx, boltEntity, checker); err != nil {
			if patch {
				pfxlog.Logger().WithError(err).Errorf("could not patch %v entity", handler.GetStore().GetEntityType())
			} else {
				pfxlog.Logger().WithError(err).Errorf("could not update %v entity", handler.GetStore().GetEntityType())
			}
			return err
		}
		return nil
	})
}

func (handler *baseHandler) readEntity(id string, modelEntity boltEntitySink) error {
	return handler.GetDb().View(func(tx *bbolt.Tx) error {
		return handler.readEntityInTx(tx, id, modelEntity)
	})
}

func (handler *baseHandler) readEntityInTx(tx *bbolt.Tx, id string, modelEntity boltEntitySink) error {
	boltEntity := handler.GetStore().NewStoreEntity()
	found, err := handler.GetStore().BaseLoadOneById(tx, id, boltEntity)
	if err != nil {
		return err
	}
	if !found {
		return boltz.NewNotFoundError(handler.GetStore().GetSingularEntityType(), "id", id)
	}

	return modelEntity.fillFrom(handler.impl, tx, boltEntity)
}

func (handler *baseHandler) readEntityWithIndex(name string, key []byte, index boltz.ReadIndex, modelEntity boltEntitySink) error {
	return handler.GetDb().View(func(tx *bbolt.Tx) error {
		return handler.readEntityInTxWithIndex(name, tx, key, index, modelEntity)
	})
}

func (handler *baseHandler) readEntityInTxWithIndex(name string, tx *bbolt.Tx, key []byte, index boltz.ReadIndex, modelEntity boltEntitySink) error {
	id := index.Read(tx, key)
	if id == nil {
		return boltz.NewNotFoundError(handler.GetStore().GetSingularEntityType(), name, string(key))
	}
	return handler.readEntityInTx(tx, string(id), modelEntity)
}

func (handler *baseHandler) readEntityByQuery(query string) (models.Entity, error) {
	result, err := handler.BaseList(query)
	if err != nil {
		return nil, err
	}
	if len(result.GetEntities()) > 0 {
		return result.GetEntities()[0], nil
	}
	return nil, nil
}

func (handler *baseHandler) deleteEntity(id string) error {
	return controllers.DeleteEntityById(handler.GetStore(), handler.GetDb(), id)
}

func (handler *baseHandler) list(queryString string, resultHandler models.ListResultHandler) error {
	return handler.GetDb().View(func(tx *bbolt.Tx) error {
		return handler.ListWithTx(tx, queryString, resultHandler)
	})
}

func (handler *baseHandler) queryRoleAttributes(index boltz.SetReadIndex, queryString string) ([]string, *models.QueryMetaData, error) {
	query, err := ast.Parse(handler.env.GetStores().Index, queryString)
	if err != nil {
		return nil, nil, err
	}

	var results []string
	var count int64
	err = handler.GetDb().View(func(tx *bbolt.Tx) error {
		results, count, err = handler.env.GetStores().Index.QueryWithCursorC(tx, index.OpenKeyCursor, query)
		return err
	})

	if err != nil {
		return nil, nil, err
	}

	qmd := &models.QueryMetaData{
		Count:            count,
		Limit:            *query.GetLimit(),
		Offset:           *query.GetSkip(),
		FilterableFields: handler.env.GetStores().Index.GetPublicSymbols(),
	}

	return results, qmd, nil
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

type FieldChecker map[string]struct{}

func NewFieldChecker(fields ...string) *FieldChecker {
	ret := FieldChecker{}
	for _, field := range fields {
		ret[field] = struct{}{}
	}

	return &ret
}

func (fc *FieldChecker) IsUpdated(field string) bool {
	_, found := (*fc)[field]
	return found
}
