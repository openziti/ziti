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
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/storage/ast"
	"go.etcd.io/bbolt"
	"time"
)

const (
	IndexesBucket = "indexes"
)

const (
	FieldId             = "id"
	FieldCreatedAt      = "createdAt"
	FieldUpdatedAt      = "updatedAt"
	FieldTags           = "tags"
	FieldIsSystemEntity = "isSystem"
)

const (
	ContextChangeAuthorIdKey   = "changeAuthorId"
	ContextChangeAuthorNameKey = "changeAuthorName"
	ContextChangeTraceId       = "traceId"
)

type EntityEventType byte

const (
	EntityCreated EntityEventType = 1
	EntityUpdated EntityEventType = 2
	EntityDeleted EntityEventType = 3
)

type Checkable interface {
	CheckIntegrity(ctx MutateContext, fix bool, errorSink func(err error, fixed bool)) error
}

type Store interface {
	ast.SymbolTypes
	Checkable
	Constrained

	GetEntityType() string
	GetSingularEntityType() string
	GetRootPath() []string
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
	GrantSymbols(child Store)
	addSymbol(name string, public bool, symbol EntitySymbol) EntitySymbol
	inheritMapSymbol(symbol *entityMapSymbol)
	AddIdSymbol(name string, nodeType ast.NodeType) EntitySymbol
	AddSymbol(name string, nodeType ast.NodeType, path ...string) EntitySymbol
	AddFkSymbol(name string, linkedType Store, path ...string) EntitySymbol
	AddSymbolWithKey(name string, nodeType ast.NodeType, key string, path ...string) EntitySymbol
	AddFkSymbolWithKey(name string, key string, linkedType Store, path ...string) EntitySymbol
	AddMapSymbol(name string, nodeType ast.NodeType, key string, path ...string)
	AddSetSymbol(name string, nodeType ast.NodeType) EntitySetSymbol
	AddPublicSetSymbol(name string, nodeType ast.NodeType) EntitySetSymbol
	AddFkSetSymbol(name string, linkedType Store) EntitySetSymbol
	NewEntitySymbol(name string, nodeType ast.NodeType) EntitySymbol
	newEntitySymbol(name string, nodeType ast.NodeType, key string, linkedType Store, prefix ...string) *entitySymbol

	AddExtEntitySymbols()
	MakeSymbolPublic(name string)

	newRowComparator(sort []ast.SortField) (RowComparator, error)
	GetPublicSymbols() []string
	IsPublicSymbol(symbol string) bool

	FindMatching(tx *bbolt.Tx, readIndex SetReadIndex, values []string) []string

	AddLinkCollection(local EntitySymbol, remove EntitySymbol) LinkCollection
	AddRefCountedLinkCollection(local EntitySymbol, remove EntitySymbol) RefCountedLinkCollection
	GetLinkCollection(name string) LinkCollection
	GetRefCountedLinkCollection(name string) RefCountedLinkCollection
	getLinks() map[string]LinkCollection

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

	GetParentStore() Store

	DeleteById(ctx MutateContext, id string) error
	DeleteWhere(ctx MutateContext, query string) error
	processDeleteConstraints(ctx MutateContext, id string) (EntityChangeFlow, error)

	NewIndexingContext(isCreate bool, ctx MutateContext, id string, holder errorz.ErrorHolder) *IndexingContext
	newEntityChangeFlow() EntityChangeFlow

	AddListener(listener func(Entity), changeType EntityEventType, changeTypes ...EntityEventType)
	AddEntityIdListener(listener func(string), changeType EntityEventType, changeTypes ...EntityEventType)
	AddUntypedEntityConstraint(constraint UntypedEntityConstraint)
}

type ChildStoreStrategy[E Entity] interface {
	HandleUpdate(ctx MutateContext, entity E, checker FieldChecker) (bool, error)
	HandleDelete(ctx MutateContext, entity E) error
	GetStore() Store
}

type EntityConstraint[E Entity] interface {
	ProcessPreCommit(state *EntityChangeState[E]) error
	ProcessPostCommit(state *EntityChangeState[E])
}

type UntypedEntityConstraint interface {
	ProcessPreCommit(state UntypedEntityChangeState) error
	ProcessPostCommit(state UntypedEntityChangeState)
}

type EntityEventListener[E Entity] interface {
	HandleEntityEvent(entity E)
}

type EntityStore[E Entity] interface {
	Store

	RegisterChildStoreStrategy(childStoreStrategy ChildStoreStrategy[E])

	Create(ctx MutateContext, entity E) error
	Update(ctx MutateContext, entity E, checker FieldChecker) error

	FindById(tx *bbolt.Tx, id string) (E, bool, error)
	FindOneByQuery(tx *bbolt.Tx, query string) (E, bool, error)
	LoadEntity(tx *bbolt.Tx, id string, entity E) (bool, error)

	GetEntityStrategy() EntityStrategy[E]
	AddEntityConstraint(constraint EntityConstraint[E])
	AddEntityEventListener(listener EntityEventListener[E], changeType EntityEventType, changeTypes ...EntityEventType)
	AddEntityEventListenerF(listener func(E), changeType EntityEventType, changeTypes ...EntityEventType)

	NewStoreEntity() E
}

type EntityStrategy[E Entity] interface {
	New() E
	LoadEntity(entity E, bucket *TypedBucket)
	PersistEntity(entity E, ctx *PersistContext)
}

type PersistContext struct {
	MutateContext
	Id           string
	Store        Store
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
	//	LoadValues(store EntityStore, bucket *TypedBucket)
	//	SetValues(ctx *PersistContext)
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

type NilEntity struct{}

func (n NilEntity) GetId() string {
	panic("NilEntity methods should never be called")
}

func (n NilEntity) SetId(string) {
	panic("NilEntity methods should never be called")
}

func (n NilEntity) GetEntityType() string {
	panic("NilEntity methods should never be called")
}
