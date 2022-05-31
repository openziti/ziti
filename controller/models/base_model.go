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

package models

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/foundation/util/errorz"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
	"time"
)

const (
	ListLimitMax      = 500
	ListOffsetMax     = 100000
	ListLimitDefault  = 10
	ListOffsetDefault = 0
)

type EntityRetriever interface {
	BaseLoad(id string) (Entity, error)
	BaseLoadInTx(tx *bbolt.Tx, id string) (Entity, error)

	BaseList(query string) (*EntityListResult, error)
	BasePreparedList(query ast.Query) (*EntityListResult, error)
	BasePreparedListAssociated(id string, typeLoader EntityRetriever, query ast.Query) (*EntityListResult, error)

	GetStore() boltz.CrudStore

	// GetEntityTypeId returns a unique id for the entity type. Some entities may share a storage type, such
	// as fabric and edge services, and fabric and edge routers. However, they should have distinct entity type
	// ids, so we can figure out to which controller to route commands
	GetEntityTypeId() string
}

type Entity interface {
	GetId() string
	SetId(string)
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
	GetTags() map[string]interface{}
	IsSystemEntity() bool
}

type BaseEntity struct {
	Id        string
	CreatedAt time.Time
	UpdatedAt time.Time
	Tags      map[string]interface{}
	IsSystem  bool
}

func (entity *BaseEntity) GetId() string {
	return entity.Id
}

func (entity *BaseEntity) SetId(id string) {
	entity.Id = id
}

func (entity *BaseEntity) GetCreatedAt() time.Time {
	return entity.CreatedAt
}

func (entity *BaseEntity) GetUpdatedAt() time.Time {
	return entity.UpdatedAt
}

func (entity *BaseEntity) GetTags() map[string]interface{} {
	return entity.Tags
}

func (entity *BaseEntity) IsSystemEntity() bool {
	return entity.IsSystem
}

func (entity *BaseEntity) FillCommon(boltEntity boltz.ExtEntity) {
	entity.Id = boltEntity.GetId()
	entity.CreatedAt = boltEntity.GetCreatedAt()
	entity.UpdatedAt = boltEntity.GetUpdatedAt()
	entity.Tags = boltEntity.GetTags()
	entity.IsSystem = boltEntity.IsSystemEntity()
}

type EntityListResult struct {
	Loader   EntityRetriever
	Entities []Entity
	QueryMetaData
}

func (result *EntityListResult) GetEntities() []Entity {
	return result.Entities
}

func (result *EntityListResult) GetMetaData() *QueryMetaData {
	return &result.QueryMetaData
}

func (result *EntityListResult) Collect(tx *bbolt.Tx, ids []string, queryMetaData *QueryMetaData) error {
	result.QueryMetaData = *queryMetaData
	for _, key := range ids {
		entity, err := result.Loader.BaseLoadInTx(tx, key)
		if err != nil {
			return err
		}
		result.Entities = append(result.Entities, entity)
	}
	return nil
}

type QueryMetaData struct {
	Count            int64
	Limit            int64
	Offset           int64
	FilterableFields []string
}

type BaseEntityManager struct {
	Store boltz.CrudStore
}

func (ctrl *BaseEntityManager) GetStore() boltz.CrudStore {
	return ctrl.Store
}

type ListResultHandler func(tx *bbolt.Tx, ids []string, qmd *QueryMetaData) error

func (ctrl *BaseEntityManager) checkLimits(query ast.Query) {
	if query.GetLimit() == nil || *query.GetLimit() < -1 || *query.GetLimit() == 0 {
		query.SetLimit(ListLimitDefault)
	} else if *query.GetLimit() > ListLimitMax {
		query.SetLimit(ListLimitMax)
	}

	if query.GetSkip() == nil || *query.GetSkip() < 0 {
		query.SetSkip(ListOffsetDefault)
	} else if *query.GetSkip() > ListOffsetMax {
		query.SetSkip(ListOffsetMax)
	}
}

func (ctrl *BaseEntityManager) ListWithTx(tx *bbolt.Tx, queryString string, resultHandler ListResultHandler) error {
	query, err := ast.Parse(ctrl.Store, queryString)
	if err != nil {
		return err
	}

	return ctrl.PreparedListWithTx(tx, query, resultHandler)
}

func (ctrl *BaseEntityManager) PreparedListWithTx(tx *bbolt.Tx, query ast.Query, resultHandler ListResultHandler) error {
	ctrl.checkLimits(query)

	keys, count, err := ctrl.Store.QueryIdsC(tx, query)
	if err != nil {
		return err
	}
	qmd := &QueryMetaData{
		Count:            count,
		Limit:            *query.GetLimit(),
		Offset:           *query.GetSkip(),
		FilterableFields: ctrl.Store.GetPublicSymbols(),
	}
	return resultHandler(tx, keys, qmd)
}

func (ctrl *BaseEntityManager) PreparedListAssociatedWithTx(tx *bbolt.Tx, id, association string, query ast.Query, resultHandler ListResultHandler) error {
	ctrl.checkLimits(query)

	var count int64
	var keys []string
	var err error

	symbol := ctrl.GetStore().GetSymbol(association)
	if symbol == nil {
		return errors.Errorf("invalid association: '%v'", association)
	}

	linkedType := symbol.GetLinkedType()
	if linkedType == nil {
		return errors.Errorf("invalid association: '%v'", association)
	}

	cursorProvider := func(tx *bbolt.Tx, forward bool) ast.SetCursor {
		return symbol.GetStore().GetRelatedEntitiesCursor(tx, id, association, forward)
	}

	if keys, count, err = linkedType.QueryWithCursorC(tx, cursorProvider, query); err != nil {
		return err
	}

	qmd := &QueryMetaData{
		Count:            count,
		Limit:            *query.GetLimit(),
		Offset:           *query.GetSkip(),
		FilterableFields: linkedType.GetPublicSymbols(),
	}
	return resultHandler(tx, keys, qmd)
}

func (ctrl *BaseEntityManager) PreparedListIndexedWithTx(tx *bbolt.Tx, cursorProvider ast.SetCursorProvider, query ast.Query, resultHandler ListResultHandler) error {
	ctrl.checkLimits(query)

	keys, count, err := ctrl.Store.QueryWithCursorC(tx, cursorProvider, query)
	if err != nil {
		return err
	}

	qmd := &QueryMetaData{
		Count:            count,
		Limit:            *query.GetLimit(),
		Offset:           *query.GetSkip(),
		FilterableFields: ctrl.Store.GetPublicSymbols(),
	}

	return resultHandler(tx, keys, qmd)
}

type Named interface {
	GetName() string
}

func (ctrl *BaseEntityManager) ValidateNameOnUpdate(ctx boltz.MutateContext, updatedEntity, existingEntity boltz.Entity, checker boltz.FieldChecker) error {
	// validate name for named entities
	if namedEntity, ok := updatedEntity.(boltz.NamedExtEntity); ok {
		existingNamed := existingEntity.(boltz.NamedExtEntity)
		if (checker == nil || checker.IsUpdated("name")) && namedEntity.GetName() != existingNamed.GetName() {
			if namedEntity.GetName() == "" {
				return errorz.NewFieldError("name is required", "name", namedEntity.GetName())
			}
			if nameIndexStore, ok := ctrl.GetStore().(db.NameIndexedStore); ok {
				if nameIndexStore.GetNameIndex().Read(ctx.Tx(), []byte(namedEntity.GetName())) != nil {
					return errorz.NewFieldError("name is must be unique", "name", namedEntity.GetName())
				}
			} else {
				pfxlog.Logger().Errorf("entity of type %v is named, but store doesn't have name index", reflect.TypeOf(updatedEntity))
			}
		}
	}
	return nil
}

func (handler *BaseEntityManager) ValidateName(db boltz.Db, boltEntity Named) error {
	return db.View(func(tx *bbolt.Tx) error {
		ctx := boltz.NewMutateContext(tx)
		return handler.ValidateNameOnCreate(ctx, boltEntity)
	})
}

func (handler *BaseEntityManager) ValidateNameOnCreate(ctx boltz.MutateContext, entity interface{}) error {
	// validate name for named entities
	if namedEntity, ok := entity.(Named); ok {
		if namedEntity.GetName() == "" {
			return errorz.NewFieldError("name is required", "name", namedEntity.GetName())
		}
		if nameIndexStore, ok := handler.GetStore().(db.NameIndexedStore); ok {
			if nameIndexStore.GetNameIndex().Read(ctx.Tx(), []byte(namedEntity.GetName())) != nil {
				return errorz.NewFieldError("name is must be unique", "name", namedEntity.GetName())
			}
		} else {
			pfxlog.Logger().Errorf("entity of type %v is named, but store doesn't have name index", reflect.TypeOf(entity))
		}
	}
	return nil
}
