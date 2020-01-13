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
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/persistence"
	"github.com/netfoundry/ziti-edge/controller/util"
	"github.com/netfoundry/ziti-foundation/storage/ast"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

type Handler interface {
	GetStore() persistence.Store
	GetDbProvider() persistence.DbProvider
	GetEnv() Env
	NewModelEntity() BaseModelEntity
	BaseList(queryOptions *QueryOptions) (*BaseModelEntityListResult, error)
	BaseLoad(id string) (BaseModelEntity, error)

	readEntityInTx(tx *bbolt.Tx, id string, modelEntity BaseModelEntity) error
}

type baseHandler struct {
	store persistence.Store
	env   Env
	impl  Handler
}

func (handler *baseHandler) GetStore() persistence.Store {
	return handler.store
}

func (handler *baseHandler) GetDbProvider() persistence.DbProvider {
	return handler.env.GetDbProvider()
}

func (handler *baseHandler) GetDb() boltz.Db {
	return handler.GetDbProvider().GetDb()
}

func (handler *baseHandler) GetEnv() Env {
	return handler.env
}

func (handler *baseHandler) BaseList(queryOptions *QueryOptions) (*BaseModelEntityListResult, error) {
	result := &BaseModelEntityListResult{handler: handler}
	err := handler.parseAndList(queryOptions, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *baseHandler) BaseLoad(id string) (BaseModelEntity, error) {
	entity := handler.impl.NewModelEntity()
	if err := handler.readEntity(id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

type BaseModelEntityListResult struct {
	handler  *baseHandler
	Entities []BaseModelEntity
	QueryMetaData
}

func (result *BaseModelEntityListResult) collect(tx *bbolt.Tx, ids []string, queryMetaData *QueryMetaData) error {
	result.QueryMetaData = *queryMetaData
	for _, key := range ids {
		entity := result.handler.impl.NewModelEntity()
		err := result.handler.readEntityInTx(tx, key, entity)
		if err != nil {
			return err
		}
		result.Entities = append(result.Entities, entity)
	}
	return nil
}

func (handler *baseHandler) createEntity(modelEntity BaseModelEntity) (string, error) {
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

func (handler *baseHandler) createEntityInTx(ctx boltz.MutateContext, modelEntity BaseModelEntity) (string, error) {
	if modelEntity == nil {
		return "", errors.Errorf("can't create %v with nil value", handler.store.GetEntityType())
	}
	if modelEntity.GetId() == "" {
		modelEntity.setId(uuid.New().String())
	}

	boltEntity, err := modelEntity.ToBoltEntityForCreate(ctx.Tx(), handler.impl)
	if err != nil {
		return "", err
	}
	store := handler.GetStore()
	if err := store.Create(ctx, boltEntity); err != nil {
		pfxlog.Logger().WithError(err).Errorf("could not create %v in bolt storage", handler.store.GetEntityType())
		return "", err
	}

	return modelEntity.GetId(), nil
}

func (handler *baseHandler) updateEntity(modelEntity BaseModelEntity, checker boltz.FieldChecker) error {
	return handler.updateGeneral(modelEntity, checker, false)
}

func (handler *baseHandler) patchEntity(modelEntity BaseModelEntity, checker boltz.FieldChecker) error {
	return handler.updateGeneral(modelEntity, checker, true)
}

func (handler *baseHandler) updateGeneral(modelEntity BaseModelEntity, checker boltz.FieldChecker, patch bool) error {
	return handler.GetDb().Update(func(tx *bbolt.Tx) error {
		ctx := boltz.NewMutateContext(tx)
		existing := handler.store.NewStoreEntity()
		found, err := handler.store.BaseLoadOneById(tx, modelEntity.GetId(), existing)
		if err != nil {
			return err
		}
		if !found {
			return util.NewNotFoundError(handler.GetStore().GetSingularEntityType(), "id", modelEntity.GetId())
		}
		var boltEntity persistence.BaseEdgeEntity
		if patch {
			boltEntity, err = modelEntity.ToBoltEntityForPatch(tx, handler.impl)
		} else {
			boltEntity, err = modelEntity.ToBoltEntityForUpdate(tx, handler.impl)
		}
		if err != nil {
			return err
		}
		if err := handler.store.Update(ctx, boltEntity, checker); err != nil {
			if patch {
				pfxlog.Logger().WithError(err).Errorf("could not patch %v entity", handler.store.GetEntityType())
			} else {
				pfxlog.Logger().WithError(err).Errorf("could not update %v entity", handler.store.GetEntityType())
			}
			return err
		}
		return nil
	})
}

func (handler *baseHandler) readEntity(id string, modelEntity BaseModelEntity) error {
	return handler.GetDb().View(func(tx *bbolt.Tx) error {
		return handler.readEntityInTx(tx, id, modelEntity)
	})
}

func (handler *baseHandler) readEntityInTx(tx *bbolt.Tx, id string, modelEntity BaseModelEntity) error {
	boltEntity := handler.store.NewStoreEntity()
	found, err := handler.store.BaseLoadOneById(tx, id, boltEntity)
	if err != nil {
		return err
	}
	if !found {
		return util.NewNotFoundError(handler.store.GetSingularEntityType(), "id", id)
	}

	return modelEntity.FillFrom(handler.impl, tx, boltEntity)
}

func (handler *baseHandler) readEntityWithIndex(name string, key []byte, index boltz.ReadIndex, modelEntity BaseModelEntity) error {
	return handler.GetDb().View(func(tx *bbolt.Tx) error {
		return handler.readEntityInTxWithIndex(name, tx, key, index, modelEntity)
	})
}

func (handler *baseHandler) readEntityInTxWithIndex(name string, tx *bbolt.Tx, key []byte, index boltz.ReadIndex, modelEntity BaseModelEntity) error {
	id := index.Read(tx, key)
	if id == nil {
		return util.NewNotFoundError(handler.store.GetSingularEntityType(), name, string(key))
	}
	return handler.readEntityInTx(tx, string(id), modelEntity)
}

func (handler *baseHandler) readEntityByQuery(query string) (BaseModelEntity, error) {
	result, err := handler.BaseList(NewQueryOptions(query, nil, ""))
	if err != nil {
		return nil, err
	}
	if len(result.Entities) > 0 {
		return result.Entities[0], nil
	}
	return nil, nil
}

func (handler *baseHandler) deleteEntity(id string, beforeDelete func(tx *bbolt.Tx, id string) error) error {
	return handler.GetDb().Update(func(tx *bbolt.Tx) error {
		ctx := boltz.NewMutateContext(tx)
		if !handler.GetStore().IsEntityPresent(tx, id) {
			return util.NewNotFoundError(handler.GetStore().GetSingularEntityType(), "id", id)
		}

		if beforeDelete != nil {
			if err := beforeDelete(tx, id); err != nil {
				return err
			}
		}

		err := handler.GetStore().DeleteById(ctx, id)

		if err != nil {
			pfxlog.Logger().WithField("id", id).WithError(err).Error("could not delete by id")
			return err
		}
		return nil
	})
}

type queryResultHandler func(tx *bbolt.Tx, ids []string, qmd *QueryMetaData) error

func (handler *baseHandler) parseAndList(queryOptions *QueryOptions, resultHandler queryResultHandler) error {
	// validate that the submitted query is only using public symbols. The query options may contain an final
	// query which has been modified with additional filters
	queryString := queryOptions.getOriginalFullQuery()
	query, err := ast.Parse(handler.GetStore(), queryString)
	if err != nil {
		return err
	}

	if err = boltz.ValidateSymbolsArePublic(query, handler.store); err != nil {
		return err
	}

	return handler.list(queryOptions.getFinalFullQuery(), resultHandler)
}

func (handler *baseHandler) list(queryString string, resultHandler queryResultHandler) error {
	return handler.GetDbProvider().GetDb().View(func(tx *bbolt.Tx) error {
		return handler.listWithTx(tx, queryString, resultHandler)
	})
}

func (handler *baseHandler) listWithTx(tx *bbolt.Tx, queryString string, resultHandler queryResultHandler) error {
	query, err := ast.Parse(handler.GetStore(), queryString)
	if err != nil {
		return err
	}

	keys, count, err := handler.GetStore().QueryIdsC(tx, query)
	if err != nil {
		return err
	}
	qmd := &QueryMetaData{
		Count:            count,
		Limit:            *query.GetLimit(),
		Offset:           *query.GetSkip(),
		FilterableFields: handler.GetStore().GetPublicSymbols(),
	}
	return resultHandler(tx, keys, qmd)
}

func (handler *baseHandler) collectAssociated(id string, field string, relatedHandler Handler, collector func(entity BaseModelEntity)) error {
	return handler.GetDb().View(func(tx *bbolt.Tx) error {
		entity := handler.impl.NewModelEntity()
		if err := handler.readEntityInTx(tx, id, entity); err != nil {
			return err
		}
		relatedEntityIds := handler.store.GetRelatedEntitiesIdList(tx, id, field)
		for _, relatedEntityId := range relatedEntityIds {
			relatedEntity := relatedHandler.NewModelEntity()
			if err := relatedHandler.readEntityInTx(tx, relatedEntityId, relatedEntity); err != nil {
				return err
			}
			collector(relatedEntity)
		}
		return nil
	})
}

type AndFieldChecker struct {
	first  boltz.FieldChecker
	second boltz.FieldChecker
}

func (checker *AndFieldChecker) IsUpdated(field string) bool {
	return checker.first.IsUpdated(field) && checker.second.IsUpdated(field)
}
