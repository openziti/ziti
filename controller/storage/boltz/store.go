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
	"reflect"
	"strings"

	"github.com/openziti/foundation/v2/concurrenz"
	"go.etcd.io/bbolt"
)

type StoreDefinition[E Entity] struct {
	EntityType      string
	EntityStrategy  EntityStrategy[E]
	BasePath        []string
	Parent          Store
	ParentMapper    func(Entity) Entity
	EntityNotFoundF func(id string) error
}

func (self *StoreDefinition[E]) WithBasePath(basePath ...string) *StoreDefinition[E] {
	self.BasePath = basePath
	return self
}

func NewBaseStore[E Entity](definition StoreDefinition[E]) *BaseStore[E] {
	if definition.EntityType == "" && definition.Parent != nil {
		definition.EntityType = definition.Parent.GetEntityType()
	}

	entityPath := append([]string{}, definition.BasePath...)
	if definition.Parent == nil {
		entityPath = append(entityPath, definition.EntityType)
	}

	indexPath := definition.BasePath
	if definition.Parent != nil {
		indexPath = definition.Parent.GetRootPath()
	}

	result := &BaseStore[E]{
		entityType:      definition.EntityType,
		entityPath:      entityPath,
		entityStrategy:  definition.EntityStrategy,
		parent:          definition.Parent,
		parentMapper:    definition.ParentMapper,
		mapSymbols:      map[string]*entityMapSymbol{},
		publicSymbols:   map[string]struct{}{},
		Indexer:         *NewIndexer(append(indexPath, IndexesBucket)...),
		links:           map[string]LinkCollection{},
		refCountedLinks: map[string]RefCountedLinkCollection{},
		entityNotFoundF: definition.EntityNotFoundF,
	}

	return result
}

type EntityChangeState[E Entity] struct {
	EventId      string
	EntityId     string
	Ctx          MutateContext
	InitialState E
	FinalState   E
	store        *BaseStore[E]
	ChangeType   EntityEventType
	ParentEvent  bool
}

func (self *EntityChangeState[E]) GetEventId() string {
	return self.EventId
}

func (self *EntityChangeState[E]) GetEntityId() string {
	return self.EntityId
}

func (self *EntityChangeState[E]) GetCtx() MutateContext {
	return self.Ctx
}

func (self *EntityChangeState[E]) GetChangeType() EntityEventType {
	return self.ChangeType
}

func (self *EntityChangeState[E]) GetInitialState() Entity {
	return self.InitialState
}

func (self *EntityChangeState[E]) GetFinalState() Entity {
	return self.FinalState
}

func (self *EntityChangeState[E]) GetStore() Store {
	return self.store
}

func (self *EntityChangeState[E]) IsParentEvent() bool {
	return self.ParentEvent
}

func (self *EntityChangeState[E]) MarkParentEvent() {
	self.ParentEvent = true
}

func (self *EntityChangeState[E]) GetInitialParentEntity() Entity {
	var tmp Entity = self.InitialState
	if tmp == nil || reflect.ValueOf(tmp).IsNil() {
		return nil
	}
	return self.store.parentMapper(self.InitialState)
}

func (self *EntityChangeState[E]) GetFinalParentEntity() Entity {
	var tmp Entity = self.FinalState
	if tmp == nil || reflect.ValueOf(tmp).IsNil() {
		return nil
	}
	return self.store.parentMapper(self.FinalState)
}

func (self *EntityChangeState[E]) initFromChild(flow entityChangeFlow) {
	self.EntityId = flow.GetEntityId()
	self.Ctx = flow.GetCtx()
	self.ChangeType = flow.GetChangeType()
	self.ParentEvent = true

	if parentEntity := flow.GetInitialParentEntity(); parentEntity != nil {
		self.InitialState = parentEntity.(E)
	}

	if parentEntity := flow.GetFinalParentEntity(); parentEntity != nil {
		self.FinalState = parentEntity.(E)
	}
}

func (self *EntityChangeState[E]) init(ctx MutateContext) (bool, error) {
	self.Ctx = ctx
	var err error
	var found bool
	self.InitialState, found, err = self.store.impl.FindById(ctx.Tx(), self.EntityId)
	return found, err
}

func (self *EntityChangeState[E]) loadFinalState() error {
	var err error
	self.FinalState, _, err = self.store.impl.FindById(self.Ctx.Tx(), self.EntityId)
	return err
}

func (self *EntityChangeState[E]) queuePostCommitProcessing(ctx MutateContext) {
	ctx.Tx().OnCommit(self.processPostCommit)
}

func (self *EntityChangeState[E]) fireEvents() error {
	if err := self.processPreCommit(); err != nil {
		return err
	}

	self.Ctx.Tx().OnCommit(self.processPostCommit)
	return nil
}

func (self *EntityChangeState[E]) processPreCommit() error {
	for _, constraint := range self.store.entityConstraints.Value() {
		if err := constraint.ProcessPreCommit(self); err != nil {
			return err
		}
	}
	return nil
}

func (self *EntityChangeState[E]) processPostCommit() {
	for _, constraint := range self.store.entityConstraints.Value() {
		constraint.ProcessPostCommit(self)
	}
}

type entityChangeFlow interface {
	UntypedEntityChangeState
	init(ctx MutateContext) (bool, error)
	initFromChild(flow entityChangeFlow)
	loadFinalState() error
	processPreCommit() error
	processPostCommit()
	fireEvents() error

	// Used for adds so that the add event gets generated before any other events coming from constraints
	queuePostCommitProcessing(ctx MutateContext)
	MarkParentEvent()
}

type BaseStore[E Entity] struct {
	entityStrategy       EntityStrategy[E]
	childStoreStrategies []ChildStoreStrategy[E]
	parent               Store
	parentMapper         func(childEntity Entity) Entity
	entityType           string
	entityPath           []string
	symbols              concurrenz.CopyOnWriteMap[string, EntitySymbol]
	publicSymbols        map[string]struct{}
	mapSymbols           map[string]*entityMapSymbol
	isExtended           bool
	Indexer
	links           map[string]LinkCollection
	refCountedLinks map[string]RefCountedLinkCollection
	entityNotFoundF func(id string) error

	entityConstraints concurrenz.CopyOnWriteSlice[EntityConstraint[E]]

	// We track the actual implementation here to ensure that when methods that are overridden from BaseStore
	// we call the override instead of the base method
	impl EntityStore[E]
}

func (store *BaseStore[E]) InitImpl(impl EntityStore[E]) {
	if store.impl == nil {
		store.impl = impl
	}
}

func (store *BaseStore[E]) RegisterChildStoreStrategy(childStoreStrategy ChildStoreStrategy[E]) {
	store.childStoreStrategies = append(store.childStoreStrategies, childStoreStrategy)
}

func (store *BaseStore[E]) GetRootPath() []string {
	if store.parent != nil {
		return store.parent.GetRootPath()
	}
	if len(store.entityPath) == 1 {
		return nil
	}
	return store.basePath[0 : len(store.entityPath)-1]
}

func (store *BaseStore[E]) GetEntityType() string {
	return store.entityType
}

func (store *BaseStore[E]) GetEntitiesBucket(tx *bbolt.Tx) *TypedBucket {
	if store.parent == nil {
		return Path(tx, store.entityPath...)
	}
	return store.parent.GetEntitiesBucket(tx)
}

func (store *BaseStore[E]) getOrCreateEntitiesBucket(tx *bbolt.Tx) *TypedBucket {
	if store.parent == nil {
		return GetOrCreatePath(tx, store.entityPath...)
	}
	return store.parent.getOrCreateEntitiesBucket(tx)
}

func (store *BaseStore[E]) GetEntityBucket(tx *bbolt.Tx, id []byte) *TypedBucket {
	baseBucket := store.GetEntitiesBucket(tx)
	entityBucket := baseBucket.GetBucket(string(id))

	if store.parent == nil {
		return entityBucket
	}

	if entityBucket == nil {
		return nil
	}
	return entityBucket.GetPath(store.entityPath...)
}

func (store *BaseStore[E]) getOrCreateEntityBucket(tx *bbolt.Tx, id []byte) *TypedBucket {
	entityBaseBucket := store.getOrCreateEntitiesBucket(tx)
	entityBucket := entityBaseBucket.GetOrCreateBucket(string(id))
	if store.parent == nil {
		return entityBucket
	}
	return entityBucket.GetOrCreatePath(store.entityPath...)
}

func (store *BaseStore[E]) GetSingularEntityType() string {
	return GetSingularEntityType(store.entityType)
}

func (store *BaseStore[E]) Extended() *BaseStore[E] {
	store.isExtended = true
	return store
}

func (store *BaseStore[E]) IsExtended() bool {
	return store.isExtended
}

func GetSingularEntityType(entityType string) string {
	if strings.HasSuffix(entityType, "ies") {
		return strings.TrimSuffix(entityType, "ies") + "y"
	}
	return strings.TrimSuffix(entityType, "s")
}
