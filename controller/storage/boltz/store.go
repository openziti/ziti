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
	"go.etcd.io/bbolt"
	"strings"
)

func NewBaseStore(entityType string, entityNotFoundF func(id string) error, basePath ...string) *BaseStore {
	return newBaseStore(nil, nil, entityType, entityNotFoundF, basePath...)
}

func NewChildBaseStore(parent CrudStore, parentMapper func(Entity) Entity, entityNotFoundF func(id string) error, basePath ...string) *BaseStore {
	return newBaseStore(parent, parentMapper, parent.GetEntityType(), entityNotFoundF, basePath...)
}

func newBaseStore(parent CrudStore, parentMapper func(Entity) Entity, entityType string, entityNotFoundF func(id string) error, basePath ...string) *BaseStore {
	entityPath := append([]string{}, basePath...)
	if parent == nil {
		entityPath = append(entityPath, entityType)
	}
	result := &BaseStore{
		parent:       parent,
		parentMapper: parentMapper,
		entityType:   entityType,
		entityPath:   entityPath,
		symbols:      map[string]EntitySymbol{},
		mapSymbols:   map[string]*entityMapSymbol{},

		Indexer:         *NewIndexer(RootBucket, IndexesBucket),
		links:           map[string]LinkCollection{},
		refCountedLinks: map[string]RefCountedLinkCollection{},
		entityNotFoundF: entityNotFoundF,
		EventEmmiter:    events.New(),
	}

	if result.parent != nil {
		// result.impl isn't initialized here, so we need to defer evaluation
		result.parent.AddDeleteHandler(func(ctx MutateContext, entityId string) error {
			entity := result.impl.NewStoreEntity()
			found, err := result.BaseLoadOneById(ctx.Tx(), entityId, entity)
			if err != nil {
				return err
			}
			if found {
				ctx.AddEvent(result.impl, EventDelete, entity)
				return result.impl.cleanupExternal(ctx, entityId)
			}
			return nil
		})

		// result.impl isn't initialized here, so we need to defer evaluation
		result.parent.AddUpdateHandler(func(ctx MutateContext, entityId string) error {
			entity := result.impl.NewStoreEntity()
			found, err := result.BaseLoadOneById(ctx.Tx(), entityId, entity)
			if err != nil {
				return err
			}
			if found {
				ctx.AddEvent(result.impl, EventUpdate, entity)
			}
			return nil
		})
	}

	return result
}

type BaseStore struct {
	parent        CrudStore
	parentMapper  func(childEntity Entity) Entity
	entityType    string
	entityPath    []string
	symbols       map[string]EntitySymbol
	publicSymbols []string
	mapSymbols    map[string]*entityMapSymbol
	isExtended    bool
	Indexer
	links           map[string]LinkCollection
	refCountedLinks map[string]RefCountedLinkCollection
	entityNotFoundF func(id string) error
	updateHandlers  EntityChangeHandlers
	deleteHandlers  EntityChangeHandlers
	events.EventEmmiter

	// We track the actual implementation here to ensure that when methods that are overridden from BaseStore
	// we call the override instead of the base method
	impl CrudStore
}

func (store *BaseStore) InitImpl(impl CrudStore) {
	if store.impl == nil {
		store.impl = impl
	}
}

func (store *BaseStore) GetEntityType() string {
	return store.entityType
}

func (store *BaseStore) GetEntitiesBucket(tx *bbolt.Tx) *TypedBucket {
	if store.parent == nil {
		return Path(tx, store.entityPath...)
	}
	return store.parent.GetEntitiesBucket(tx)
}

func (store *BaseStore) GetOrCreateEntitiesBucket(tx *bbolt.Tx) *TypedBucket {
	if store.parent == nil {
		return GetOrCreatePath(tx, store.entityPath...)
	}
	return store.parent.GetOrCreateEntitiesBucket(tx)
}

func (store *BaseStore) GetEntityBucket(tx *bbolt.Tx, id []byte) *TypedBucket {
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

func (store *BaseStore) GetOrCreateEntityBucket(tx *bbolt.Tx, id []byte) *TypedBucket {
	entityBaseBucket := store.GetOrCreateEntitiesBucket(tx)
	entityBucket := entityBaseBucket.GetOrCreateBucket(string(id))
	if store.parent == nil {
		return entityBucket
	}
	return entityBucket.GetOrCreatePath(store.entityPath...)
}

func (store *BaseStore) GetValue(tx *bbolt.Tx, id []byte, path ...string) []byte {
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

func (store *BaseStore) GetValueCursor(tx *bbolt.Tx, id []byte, path ...string) *bbolt.Cursor {
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

func (store *BaseStore) AddUpdateHandler(handler EntityChangeHandler) {
	if store.parent != nil {
		store.parent.AddUpdateHandler(handler)
	}
	store.updateHandlers.Add(handler)
}

func (store *BaseStore) AddDeleteHandler(handler EntityChangeHandler) {
	if store.parent != nil {
		store.parent.AddDeleteHandler(handler)
	} else {
		store.deleteHandlers.Add(handler)
	}
}

func (store *BaseStore) GetSingularEntityType() string {
	return GetSingularEntityType(store.entityType)
}

func (store *BaseStore) Extended() *BaseStore {
	store.isExtended = true
	return store
}

func (store *BaseStore) IsExtended() bool {
	return store.isExtended
}

func GetSingularEntityType(entityType string) string {
	if strings.HasSuffix(entityType, "ies") {
		return strings.TrimSuffix(entityType, "ies") + "y"
	}
	return strings.TrimSuffix(entityType, "s")
}
