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

package persistence

import (
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/foundation/storage/ast"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/openziti/foundation/util/errorz"
	"go.etcd.io/bbolt"
	"strings"
)

type DbProvider interface {
	GetDb() boltz.Db
	GetStores() *db.Stores
	GetControllers() *network.Controllers
}

type Store interface {
	boltz.CrudStore
	initializeLocal()
	initializeLinked()
	initializeIndexes(tx *bbolt.Tx, errorHolder errorz.ErrorHolder)
}

func newBaseStore(stores *stores, entityType string) *baseStore {
	singularEntityType := boltz.GetSingularEntityType(entityType)
	return &baseStore{
		stores: stores,
		BaseStore: boltz.NewBaseStore(entityType, func(id string) error {
			return boltz.NewNotFoundError(singularEntityType, "id", id)
		}, boltz.RootBucket),
	}
}

func newChildBaseStore(stores *stores, parent boltz.CrudStore, parentMapper func(entity boltz.Entity) boltz.Entity) *baseStore {
	return newChildBaseStoreWithPath(stores, parent, parentMapper, EdgeBucket)
}

func newChildBaseStoreWithPath(stores *stores, parent boltz.CrudStore, parentMapper func(entity boltz.Entity) boltz.Entity, path string) *baseStore {
	entityNotFoundF := func(id string) error {
		return boltz.NewNotFoundError(parent.GetSingularEntityType(), "id", id)
	}

	return &baseStore{
		stores:    stores,
		BaseStore: boltz.NewChildBaseStore(parent, parentMapper, entityNotFoundF, path),
	}
}

func newExtendedBaseStore(stores *stores, parent boltz.CrudStore, parentMapper func(entity boltz.Entity) boltz.Entity, path string) *baseStore {
	store := newChildBaseStoreWithPath(stores, parent, parentMapper, path)
	store.BaseStore.Extended()
	return store
}

type baseStore struct {
	stores *stores
	*boltz.BaseStore
}

func (store *baseStore) addUniqueNameField() boltz.ReadIndex {
	symbolName := store.AddSymbol(FieldName, ast.NodeTypeString)
	return store.AddUniqueIndex(symbolName)
}

func (store *baseStore) initializeIndexes(tx *bbolt.Tx, errorHolder errorz.ErrorHolder) {
	store.InitializeIndexes(tx, errorHolder)
}

func (store *baseStore) baseLoadOneById(tx *bbolt.Tx, id string, entity boltz.Entity) error {
	found, err := store.BaseLoadOneById(tx, id, entity)
	if err != nil {
		return err
	}
	if !found {
		return boltz.NewNotFoundError(store.GetSingularEntityType(), "id", id)
	}
	return nil
}

func (store *baseStore) deleteEntityReferences(tx *bbolt.Tx, entity boltz.NamedExtEntity, rolesSymbol boltz.EntitySetSymbol) error {
	idRef := entityRef(entity.GetId())

	for _, policyHolderId := range store.GetRelatedEntitiesIdList(tx, entity.GetId(), rolesSymbol.GetStore().GetEntityType()) {
		err := rolesSymbol.Map(tx, []byte(policyHolderId), func(ctx *boltz.MapContext) {
			if ctx.ValueS() == idRef {
				ctx.Delete()
			}
		})
		if err != nil {
			return err
		}
	}
	return nil
}

type NameIndexedStore interface {
	Store
	GetNameIndex() boltz.ReadIndex
}

func (store *baseStore) GetName(tx *bbolt.Tx, id string) *string {
	symbol := store.GetSymbol(FieldName)
	if symbol == nil {
		return nil
	}
	_, val := symbol.Eval(tx, []byte(id))
	if val != nil {
		result := string(val)
		return &result
	}
	return nil
}

func (store *baseStore) getRoleAttributesCursorProvider(index boltz.SetReadIndex, values []string, semantic string) (ast.SetCursorProvider, error) {
	if semantic == "" {
		semantic = SemanticAllOf
	}

	if !isSemanticValid(semantic) {
		return nil, errorz.NewFieldError("invalid semantic", FieldSemantic, semantic)
	}

	roles, ids, err := splitRolesAndIds(values)
	if err != nil {
		return nil, err
	}

	return func(tx *bbolt.Tx, forward bool) ast.SetCursor {
		validIds := ast.NewTreeSet(forward)
		for _, id := range ids {
			if store.IsEntityPresent(tx, id) {
				validIds.Add([]byte(id))
			}
		}

		var rolesCursor ast.SetCursor
		if strings.EqualFold(semantic, SemanticAllOf) {
			rolesCursor = store.IteratorMatchingAllOf(index, roles)(tx, forward)
		} else {
			rolesCursor = store.IteratorMatchingAnyOf(index, roles)(tx, forward)
		}
		if validIds.Size() == 0 {
			return rolesCursor
		}
		return ast.NewUnionSetCursor(rolesCursor, validIds.ToCursor(), forward)
	}, nil
}
