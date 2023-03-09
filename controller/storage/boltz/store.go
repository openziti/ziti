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
	"github.com/openziti/foundation/v2/concurrenz"
	"go.etcd.io/bbolt"
	"strings"
)

type StoreDefinition[E Entity] struct {
	EntityType      string
	EntityStrategy  EntityStrategy[E]
	BasePath        []string
	Parent          CrudBaseStore
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
		symbols:         map[string]EntitySymbol{},
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
	Id           string
	Ctx          MutateContext
	InitialState E
	FinalState   E
	store        *BaseStore[E]
}

func (self *EntityChangeState[E]) Init(ctx MutateContext) (bool, error) {
	self.Ctx = ctx
	var err error
	var found bool
	self.InitialState, found, err = self.store.impl.FindById(ctx.Tx(), self.Id)
	return found, err
}

func (self *EntityChangeState[E]) LoadFinalState() error {
	var err error
	self.FinalState, _, err = self.store.impl.FindById(self.Ctx.Tx(), self.Id)
	return err
}

func (self *EntityChangeState[E]) ProcessPreCommit() error {
	for _, constraint := range self.store.entityConstraints.Value() {
		if err := constraint.ProcessPreCommit(self); err != nil {
			return err
		}
	}
	return nil
}

func (self *EntityChangeState[E]) ProcessPostCommit() {
	for _, constraint := range self.store.entityConstraints.Value() {
		constraint.ProcessPostCommit(self)
	}
}

type EntityChangeFlow interface {
	Init(ctx MutateContext) (bool, error)
	LoadFinalState() error
	ProcessPreCommit() error
	ProcessPostCommit()
}

type EntityConstraint[E Entity] interface {
	ProcessPreCommit(state *EntityChangeState[E]) error
	ProcessPostCommit(state *EntityChangeState[E])
}

type BaseStore[E Entity] struct {
	entityStrategy     EntityStrategy[E]
	childStoreStragies []ChildStoreStrategy[E]
	parent             CrudBaseStore
	parentMapper       func(childEntity Entity) Entity
	entityType         string
	entityPath         []string
	symbols            map[string]EntitySymbol
	publicSymbols      map[string]struct{}
	mapSymbols         map[string]*entityMapSymbol
	isExtended         bool
	Indexer
	links           map[string]LinkCollection
	refCountedLinks map[string]RefCountedLinkCollection
	entityNotFoundF func(id string) error

	entityConstraints concurrenz.CopyOnWriteSlice[EntityConstraint[E]]

	// We track the actual implementation here to ensure that when methods that are overridden from BaseStore
	// we call the override instead of the base method
	impl CrudStore[E]
}

func (store *BaseStore[E]) InitImpl(impl CrudStore[E]) {
	if store.impl == nil {
		store.impl = impl
	}
}

func (store *BaseStore[E]) RegisterChildStoreStrategy(childStoreStrategy ChildStoreStrategy[E]) {
	store.childStoreStragies = append(store.childStoreStragies, childStoreStrategy)
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

func (store *BaseStore[E]) GetOrCreateEntitiesBucket(tx *bbolt.Tx) *TypedBucket {
	if store.parent == nil {
		return GetOrCreatePath(tx, store.entityPath...)
	}
	return store.parent.GetOrCreateEntitiesBucket(tx)
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

func (store *BaseStore[E]) GetOrCreateEntityBucket(tx *bbolt.Tx, id []byte) *TypedBucket {
	entityBaseBucket := store.GetOrCreateEntitiesBucket(tx)
	entityBucket := entityBaseBucket.GetOrCreateBucket(string(id))
	if store.parent == nil {
		return entityBucket
	}
	return entityBucket.GetOrCreatePath(store.entityPath...)
}

func (store *BaseStore[E]) GetValue(tx *bbolt.Tx, id []byte, path ...string) []byte {
	entityBucket := store.GetEntityBucket(tx, id)
	if entityBucket == nil {
		return nil
	}
	if len(path) == 0 {
		return id
	}
	if len(path) == 1 {
		return entityBucket.Get([]byte(path[0]))
	}
	valueBucket := entityBucket.GetPath(path[:len(path)-1]...)
	if valueBucket == nil {
		return nil
	}
	return valueBucket.Get([]byte(path[len(path)-1]))
}

func (store *BaseStore[E]) GetValueCursor(tx *bbolt.Tx, id []byte, path ...string) *bbolt.Cursor {
	entityBucket := store.GetEntityBucket(tx, id)
	if entityBucket == nil {
		return nil
	}
	bucket := entityBucket.GetPath(path...)
	if bucket == nil {
		return nil
	}
	return bucket.Cursor()
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
