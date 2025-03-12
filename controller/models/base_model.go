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

package models

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/errorz"
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

type EntityRetriever[T Entity] interface {
	BaseLoad(id string) (T, error)
	BaseLoadInTx(tx *bbolt.Tx, id string) (T, error)

	BaseList(query string) (*EntityListResult[T], error)
	BasePreparedList(query ast.Query) (*EntityListResult[T], error)

	ListWithHandler(query string, handler ListResultHandler) error
	PreparedListWithHandler(query ast.Query, handler ListResultHandler) error

	PreparedListAssociatedWithHandler(id string, association string, query ast.Query, handler ListResultHandler) error

	GetListStore() boltz.Store

	// GetEntityTypeId returns a unique id for the entity type. Some entities may share a storage type, such
	// as fabric and edge services, and fabric and edge routers. However, they should have distinct entity type
	// ids, so we can figure out to which controller to route commands
	GetEntityTypeId() string

	IsEntityPresent(id string) (bool, error)
}

type Entity interface {
	GetId() string
	SetId(string)
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
	GetTags() map[string]interface{}
	IsSystemEntity() bool
}

type NameIndexedStore interface {
	boltz.Store
	GetNameIndex() boltz.ReadIndex
}

type BaseEntity struct {
	//json tags used by dynamic struct protobuff marshalling
	Id        string                 `json:"id"`
	CreatedAt time.Time              `json:"createdAt"`
	UpdatedAt time.Time              `json:"updatedAt"`
	Tags      map[string]interface{} `json:"tags"`
	IsSystem  bool                   `json:"isSystem"`
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

func (entity *BaseEntity) ToBoltBaseExtEntity() *boltz.BaseExtEntity {
	return &boltz.BaseExtEntity{
		Id:        entity.Id,
		CreatedAt: entity.CreatedAt,
		UpdatedAt: entity.UpdatedAt,
		Tags:      entity.Tags,
		IsSystem:  entity.IsSystem,
	}
}

type EntityListResult[T Entity] struct {
	Loader interface {
		BaseLoadInTx(tx *bbolt.Tx, id string) (T, error)
	}
	Entities []T
	QueryMetaData
}

func (result *EntityListResult[T]) GetEntities() []T {
	return result.Entities
}

func (result *EntityListResult[T]) GetMetaData() *QueryMetaData {
	return &result.QueryMetaData
}

func (result *EntityListResult[T]) Collect(tx *bbolt.Tx, ids []string, queryMetaData *QueryMetaData) error {
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

type BaseEntityManager[E boltz.ExtEntity] struct {
	Store boltz.EntityStore[E]
}

func (ctrl *BaseEntityManager[E]) GetStore() boltz.EntityStore[E] {
	return ctrl.Store
}

func (ctrl *BaseEntityManager[E]) GetListStore() boltz.Store {
	return ctrl.Store
}

type ListResultHandler func(tx *bbolt.Tx, ids []string, qmd *QueryMetaData) error

func (ctrl *BaseEntityManager[E]) checkLimits(query ast.Query) {
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

func (ctrl *BaseEntityManager[E]) ListWithTx(tx *bbolt.Tx, queryString string, resultHandler ListResultHandler) error {
	query, err := ast.Parse(ctrl.Store, queryString)
	if err != nil {
		return err
	}

	return ctrl.PreparedListWithTx(tx, query, resultHandler)
}

func (ctrl *BaseEntityManager[E]) PreparedListWithTx(tx *bbolt.Tx, query ast.Query, resultHandler ListResultHandler) error {
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

func (ctrl *BaseEntityManager[E]) PreparedListAssociatedWithTx(tx *bbolt.Tx, id, association string, query ast.Query, resultHandler ListResultHandler) error {
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

func (ctrl *BaseEntityManager[E]) PreparedListIndexedWithTx(tx *bbolt.Tx, cursorProvider ast.SetCursorProvider, query ast.Query, resultHandler ListResultHandler) error {
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

func (ctrl *BaseEntityManager[E]) ValidateNameOnUpdate(ctx boltz.MutateContext, updatedEntity, existingEntity boltz.Entity, checker boltz.FieldChecker) error {
	// validate name for named entities
	if namedEntity, ok := updatedEntity.(boltz.NamedExtEntity); ok {
		existingNamed := existingEntity.(boltz.NamedExtEntity)
		if (checker == nil || checker.IsUpdated("name")) && namedEntity.GetName() != existingNamed.GetName() {
			if namedEntity.GetName() == "" {
				return errorz.NewFieldError("name is required", "name", namedEntity.GetName())
			}
			if nameIndexStore, ok := ctrl.GetStore().(NameIndexedStore); ok {
				if nameIndexStore.GetNameIndex().Read(ctx.Tx(), []byte(namedEntity.GetName())) != nil {
					return errorz.NewFieldError("name must be unique", "name", namedEntity.GetName())
				}
			} else {
				pfxlog.Logger().Errorf("entity of type %v is named, but store doesn't implement the NamedIndexStore interface", reflect.TypeOf(updatedEntity))
			}
		}
	}
	return nil
}

func (handler *BaseEntityManager[E]) ValidateName(db boltz.Db, boltEntity Named) error {
	return db.View(func(tx *bbolt.Tx) error {
		return handler.ValidateNameOnCreate(tx, boltEntity)
	})
}

func (handler *BaseEntityManager[E]) ValidateNameOnCreate(tx *bbolt.Tx, entity interface{}) error {
	// validate name for named entities
	if namedEntity, ok := entity.(Named); ok {
		if namedEntity.GetName() == "" {
			return errorz.NewFieldError("name is required", "name", namedEntity.GetName())
		}
		if nameIndexStore, ok := handler.GetStore().(NameIndexedStore); ok {
			if nameIndexStore.GetNameIndex().Read(tx, []byte(namedEntity.GetName())) != nil {
				return errorz.NewFieldError("name must be unique", "name", namedEntity.GetName())
			}
		} else {
			pfxlog.Logger().Errorf("entity of type %v is named, but store doesn't implement the NamedIndexStore interface", reflect.TypeOf(entity))
		}
	}
	return nil
}
