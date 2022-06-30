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

package boltz

import (
	"github.com/kataras/go-events"
	"github.com/openziti/storage/ast"
	"github.com/openziti/foundation/v2/errorz"
	"go.etcd.io/bbolt"
	"io"
	"time"
)

const (
	RootBucket    = "ziti"
	IndexesBucket = "indexes"
)

const (
	EventCreate events.EventName = "CREATE"
	EventDelete events.EventName = "DELETE"
	EventUpdate events.EventName = "UPDATE"

	FieldId             = "id"
	FieldCreatedAt      = "createdAt"
	FieldUpdatedAt      = "updatedAt"
	FieldTags           = "tags"
	FieldIsSystemEntity = "isSystem"
)

type Db interface {
	io.Closer
	Update(fn func(tx *bbolt.Tx) error) error
	Batch(fn func(tx *bbolt.Tx) error) error
	View(fn func(tx *bbolt.Tx) error) error
	RootBucket(tx *bbolt.Tx) (*bbolt.Bucket, error)

	// Snapshot makes a copy of the bolt file
	Snapshot(tx *bbolt.Tx) error
}

type ListStore interface {
	ast.SymbolTypes

	GetEntityType() string
	GetSingularEntityType() string
	GetEntitiesBucket(tx *bbolt.Tx) *TypedBucket
	GetOrCreateEntitiesBucket(tx *bbolt.Tx) *TypedBucket
	GetEntityBucket(tx *bbolt.Tx, id []byte) *TypedBucket
	GetOrCreateEntityBucket(tx *bbolt.Tx, id []byte) *TypedBucket
	GetValue(tx *bbolt.Tx, id []byte, path ...string) []byte
	GetValueCursor(tx *bbolt.Tx, id []byte, path ...string) *bbolt.Cursor
	IsChildStore() bool
	IsEntityPresent(tx *bbolt.Tx, id string) bool
	IsExtended() bool

	GetSymbol(name string) EntitySymbol
	MapSymbol(name string, wrapper SymbolMapper)
	GrantSymbols(child ListStore)
	addSymbol(name string, public bool, symbol EntitySymbol) EntitySymbol
	AddIdSymbol(name string, nodeType ast.NodeType) EntitySymbol
	AddSymbol(name string, nodeType ast.NodeType, path ...string) EntitySymbol
	AddFkSymbol(name string, linkedType ListStore, path ...string) EntitySymbol
	AddSymbolWithKey(name string, nodeType ast.NodeType, key string, path ...string) EntitySymbol
	AddFkSymbolWithKey(name string, key string, linkedType ListStore, path ...string) EntitySymbol
	AddMapSymbol(name string, nodeType ast.NodeType, key string, path ...string)
	AddSetSymbol(name string, nodeType ast.NodeType) EntitySetSymbol
	AddPublicSetSymbol(name string, nodeType ast.NodeType) EntitySetSymbol
	AddFkSetSymbol(name string, linkedType ListStore) EntitySetSymbol
	NewEntitySymbol(name string, nodeType ast.NodeType) EntitySymbol
	AddExtEntitySymbols()
	MakeSymbolPublic(name string)

	NewRowComparator(sort []ast.SortField) (RowComparator, error)
	GetPublicSymbols() []string

	FindMatching(tx *bbolt.Tx, readIndex SetReadIndex, values []string) []string

	GetRelatedEntitiesIdList(tx *bbolt.Tx, id string, field string) []string
	GetRelatedEntitiesCursor(tx *bbolt.Tx, id string, field string, forward bool) ast.SetCursor
	IsEntityRelated(tx *bbolt.Tx, id string, field string, relatedEntityId string) bool

	// QueryIds compiles the query and runs it against the store
	QueryIds(tx *bbolt.Tx, query string) ([]string, int64, error)

	// QueryIdsf compiles the query with the given params and runs it against the store
	QueryIdsf(tx *bbolt.Tx, query string, args ...interface{}) ([]string, int64, error)

	// QueryIdsC executes a compile query against the store
	QueryIdsC(tx *bbolt.Tx, query ast.Query) ([]string, int64, error)

	QueryWithCursorC(tx *bbolt.Tx, cursorProvider ast.SetCursorProvider, query ast.Query) ([]string, int64, error)

	IterateIds(tx *bbolt.Tx, filter ast.BoolNode) ast.SeekableSetCursor

	// IterateValidIds skips non-present entities in extended stores
	IterateValidIds(tx *bbolt.Tx, filter ast.BoolNode) ast.SeekableSetCursor
}

type CrudStore interface {
	ListStore
	Constrained

	GetParentStore() CrudStore
	AddLinkCollection(local EntitySymbol, remove EntitySymbol) LinkCollection
	AddRefCountedLinkCollection(local EntitySymbol, remove EntitySymbol) RefCountedLinkCollection
	GetLinkCollection(name string) LinkCollection
	GetRefCountedLinkCollection(name string) RefCountedLinkCollection

	Create(ctx MutateContext, entity Entity) error
	Update(ctx MutateContext, entity Entity, checker FieldChecker) error
	DeleteById(ctx MutateContext, id string) error
	DeleteWhere(ctx MutateContext, query string) error
	cleanupExternal(ctx MutateContext, id string) error

	CreateChild(ctx MutateContext, parentId string, entity Entity) error
	UpdateChild(ctx MutateContext, parentId string, entity Entity, checker FieldChecker) error
	DeleteChild(ctx MutateContext, parentId string, entity Entity) error
	ListChildIds(tx *bbolt.Tx, parentId string, childType string) []string

	BaseLoadOneById(tx *bbolt.Tx, id string, entity Entity) (bool, error)
	BaseLoadOneByQuery(tx *bbolt.Tx, query string, entity Entity) (bool, error)
	BaseLoadOneChildById(tx *bbolt.Tx, id string, childId string, entity Entity) (bool, error)
	NewStoreEntity() Entity

	AddDeleteHandler(handler EntityChangeHandler)
	AddUpdateHandler(handler EntityChangeHandler)
	NewIndexingContext(isCreate bool, ctx MutateContext, id string, holder errorz.ErrorHolder) *IndexingContext

	CheckIntegrity(tx *bbolt.Tx, fix bool, errorSink func(err error, fixed bool)) error

	AddEvent(ctx MutateContext, entity Entity, name events.EventName)
	events.EventEmmiter
}

type PersistContext struct {
	MutateContext
	Id           string
	Store        CrudStore
	Bucket       *TypedBucket
	FieldChecker FieldChecker
	IsCreate     bool
}

func (ctx *PersistContext) GetParentContext() *PersistContext {
	result := &PersistContext{
		MutateContext: ctx.MutateContext,
		Id:            ctx.Id,
		Store:         ctx.Store.GetParentStore(),
		Bucket:        ctx.Store.GetParentStore().GetEntityBucket(ctx.Bucket.Tx(), []byte(ctx.Id)),
		FieldChecker:  ctx.FieldChecker,
		IsCreate:      ctx.IsCreate,
	}
	// inherit error context
	result.Bucket.ErrorHolderImpl = ctx.Bucket.ErrorHolderImpl
	return result
}

func (ctx *PersistContext) WithFieldOverrides(overrides map[string]string) {
	if ctx.FieldChecker != nil {
		ctx.FieldChecker = NewMappedFieldChecker(ctx.FieldChecker, overrides)
	}
}

func (ctx *PersistContext) GetAndSetString(field string, value string) (*string, bool) {
	return ctx.Bucket.GetAndSetString(field, value, ctx.FieldChecker)
}

func (ctx *PersistContext) SetRequiredString(field string, value string) {
	if ctx.ProceedWithSet(field) {
		if value == "" {
			ctx.Bucket.SetError(errorz.NewFieldError(field+" is required", field, value))
			return
		}
		ctx.Bucket.setTyped(TypeString, field, []byte(value))
	}
}

func (ctx *PersistContext) SetString(field string, value string) {
	ctx.Bucket.SetString(field, value, ctx.FieldChecker)
}

func (ctx *PersistContext) SetStringP(field string, value *string) {
	ctx.Bucket.SetStringP(field, value, ctx.FieldChecker)
}

func (ctx *PersistContext) SetTimeP(field string, value *time.Time) {
	ctx.Bucket.SetTimeP(field, value, ctx.FieldChecker)
}

func (ctx *PersistContext) SetBool(field string, value bool) {
	ctx.Bucket.SetBool(field, value, ctx.FieldChecker)
}

func (ctx *PersistContext) SetInt32(field string, value int32) {
	ctx.Bucket.SetInt32(field, value, ctx.FieldChecker)
}

func (ctx *PersistContext) SetInt64(field string, value int64) {
	ctx.Bucket.SetInt64(field, value, ctx.FieldChecker)
}

func (ctx *PersistContext) SetMap(field string, value map[string]interface{}) {
	ctx.Bucket.PutMap(field, value, ctx.FieldChecker, true)
}

func (ctx *PersistContext) SetStringList(field string, value []string) {
	ctx.Bucket.SetStringList(field, value, ctx.FieldChecker)
}

func (ctx *PersistContext) GetAndSetStringList(field string, value []string) ([]string, bool) {
	return ctx.Bucket.GetAndSetStringList(field, value, ctx.FieldChecker)
}

func (ctx *PersistContext) SetLinkedIds(field string, value []string) {
	if ctx.ProceedWithSet(field) {
		collection := ctx.Store.GetLinkCollection(field)
		ctx.Bucket.SetError(collection.SetLinks(ctx.Bucket.Tx(), ctx.Id, value))
	}
}

func (ctx *PersistContext) ProceedWithSet(field string) bool {
	return ctx.Bucket.ProceedWithSet(field, ctx.FieldChecker)
}

type Entity interface {
	GetId() string
	SetId(id string)
	LoadValues(store CrudStore, bucket *TypedBucket)
	SetValues(ctx *PersistContext)
	GetEntityType() string
}

type ExtEntity interface {
	Entity
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
	GetTags() map[string]interface{}
	IsSystemEntity() bool

	SetCreatedAt(createdAt time.Time)
	SetUpdatedAt(updatedAt time.Time)
	SetTags(tags map[string]interface{})
}

type NamedExtEntity interface {
	ExtEntity
	GetName() string
}

func NewExtEntity(id string, tags map[string]interface{}) *BaseExtEntity {
	return &BaseExtEntity{
		Id:   id,
		Tags: tags,
	}
}

type BaseExtEntity struct {
	Id        string
	CreatedAt time.Time
	UpdatedAt time.Time
	Tags      map[string]interface{}
	IsSystem  bool
	Migrate   bool
}

func (entity *BaseExtEntity) GetId() string {
	return entity.Id
}

func (entity *BaseExtEntity) SetId(id string) {
	entity.Id = id
}

func (entity *BaseExtEntity) GetCreatedAt() time.Time {
	return entity.CreatedAt
}

func (entity *BaseExtEntity) GetUpdatedAt() time.Time {
	return entity.UpdatedAt
}

func (entity *BaseExtEntity) GetTags() map[string]interface{} {
	return entity.Tags
}

func (entity *BaseExtEntity) IsSystemEntity() bool {
	return entity.IsSystem
}

func (entity *BaseExtEntity) SetCreatedAt(createdAt time.Time) {
	entity.CreatedAt = createdAt
}

func (entity *BaseExtEntity) SetUpdatedAt(updatedAt time.Time) {
	entity.UpdatedAt = updatedAt
}

func (entity *BaseExtEntity) SetTags(tags map[string]interface{}) {
	entity.Tags = tags
}

func (entity *BaseExtEntity) LoadBaseValues(bucket *TypedBucket) {
	entity.CreatedAt = bucket.GetTimeOrError(FieldCreatedAt)
	entity.UpdatedAt = bucket.GetTimeOrError(FieldUpdatedAt)
	entity.Tags = bucket.GetMap(FieldTags)
	entity.IsSystem = bucket.GetBoolWithDefault(FieldIsSystemEntity, false)
}

func (entity *BaseExtEntity) SetBaseValues(ctx *PersistContext) {
	if ctx.IsCreate {
		entity.CreateBaseValues(ctx)
	} else {
		entity.UpdateBaseValues(ctx)
	}
}

func (entity *BaseExtEntity) CreateBaseValues(ctx *PersistContext) {
	now := time.Now()
	if entity.Migrate {
		ctx.Bucket.SetTimeP(FieldCreatedAt, &entity.CreatedAt, nil)
		ctx.Bucket.SetTimeP(FieldUpdatedAt, &entity.UpdatedAt, nil)
	} else {
		ctx.Bucket.SetTimeP(FieldCreatedAt, &now, nil)
		ctx.Bucket.SetTimeP(FieldUpdatedAt, &now, nil)
	}
	ctx.Bucket.PutMap(FieldTags, entity.Tags, nil, false)
	if entity.IsSystem {
		ctx.Bucket.SetBool(FieldIsSystemEntity, true, nil)
	}
}

func (entity *BaseExtEntity) UpdateBaseValues(ctx *PersistContext) {
	now := time.Now()
	ctx.Bucket.SetTimeP(FieldUpdatedAt, &now, nil)
	ctx.Bucket.PutMap(FieldTags, entity.Tags, ctx.FieldChecker, false)
}
