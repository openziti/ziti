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
	"fmt"
	"github.com/netfoundry/ziti-edge/controller/validation"
	"github.com/netfoundry/ziti-fabric/controller/db"
	"github.com/netfoundry/ziti-fabric/controller/network"
	"github.com/netfoundry/ziti-foundation/storage/ast"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/netfoundry/ziti-foundation/util/errorz"
	"github.com/netfoundry/ziti-foundation/util/stringz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"strings"
)

type DbProvider interface {
	GetDb() boltz.Db
	GetServiceCache() network.Cache
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
		BaseStore: boltz.NewBaseStore(nil, entityType, func(id string) error {
			return boltz.NewNotFoundError(singularEntityType, "id", id)
		}, boltz.RootBucket),
	}
}

func newChildBaseStore(stores *stores, parent boltz.CrudStore, entityType string) *baseStore {
	singularEntityType := boltz.GetSingularEntityType(entityType)
	return &baseStore{
		stores: stores,
		BaseStore: boltz.NewBaseStore(parent, entityType, func(id string) error {
			return boltz.NewNotFoundError(singularEntityType, "id", id)
		}, EdgeBucket),
	}
}

type baseStore struct {
	stores *stores
	*boltz.BaseStore
}

func (store *baseStore) addUniqueNameField() boltz.ReadIndex {
	symbolName := store.AddSymbol(FieldName, ast.NodeTypeString)
	return store.AddUniqueIndex(symbolName)
}

func (store *baseStore) addRoleAttributesField() boltz.SetReadIndex {
	symbol := store.AddSetSymbol(FieldRoleAttributes, ast.NodeTypeString)
	return store.AddSetIndex(symbol)
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
	// If the entity name is the same as entity ID, don't remove any of those references as id references take precedence
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

func (store *baseStore) getEntityIdsForRoleSet(tx *bbolt.Tx, field string, roleSet []string, semantic string, index boltz.SetReadIndex, targetStore NameIndexedStore) ([]string, error) {
	entityStore := index.GetSymbol().GetStore()
	if stringz.Contains(roleSet, AllRole) {
		ids, _, err := entityStore.QueryIds(tx, "true")
		if err != nil {
			return nil, err
		}
		return ids, nil
	}

	roles, ids, err := splitRolesAndIds(roleSet)
	if err != nil {
		return nil, err
	}
	if err := validateEntityIds(tx, targetStore, field, ids); err != nil {
		return nil, err
	}
	var roleIds []string
	if strings.EqualFold(semantic, SemanticAllOf) {
		roleIds = store.FindMatching(tx, index, roles)
	} else if strings.EqualFold(semantic, SemanticAnyOf) {
		roleIds = store.FindMatchingAnyOf(tx, index, roles)
	} else {
		return nil, errors.Errorf("unsupported policy semantic %v", semantic)
	}

	for _, id := range ids {
		if entityStore.IsEntityPresent(tx, id) {
			roleIds = append(roleIds, id)
		}
	}
	return roleIds, nil
}

func validateEntityIds(tx *bbolt.Tx, store NameIndexedStore, field string, ids []string) error {
	var invalid []string
	for _, val := range ids {
		if !store.IsEntityPresent(tx, val) {
			invalid = append(invalid, val)
		}
	}
	if len(invalid) > 0 {
		return validation.NewFieldError(fmt.Sprintf("no %v found with the given ids", store.GetEntityType()), field, invalid)
	}
	return nil
}

func UpdateRelatedRoles(tx *bbolt.Tx, entityId string, roleSymbol boltz.EntitySetSymbol,
	linkCollection boltz.LinkCollection, new []boltz.FieldTypeAndValue, holder errorz.ErrorHolder, semanticSymbol boltz.EntitySymbol) {
	ids, _, err := roleSymbol.GetStore().QueryIds(tx, "true")
	holder.SetError(err)

	entityRoles := FieldValuesToIds(new)

	for _, id := range ids {
		roleSet := roleSymbol.EvalStringList(tx, []byte(id))
		roles, ids, err := splitRolesAndIds(roleSet)
		if err != nil {
			holder.SetError(err)
			return
		}
		semantic := SemanticAllOf
		if semanticSymbol != nil {
			if _, semanticValue := semanticSymbol.Eval(tx, []byte(id)); semanticValue != nil {
				semantic = string(semanticValue)
			}
		}

		if stringz.Contains(ids, entityId) || stringz.Contains(roles, "all") {
			err = linkCollection.AddLinks(tx, id, entityId)
		} else if strings.EqualFold(semantic, SemanticAllOf) && len(roles) > 0 && stringz.ContainsAll(entityRoles, roles...) {
			err = linkCollection.AddLinks(tx, id, entityId)
		} else if strings.EqualFold(semantic, SemanticAnyOf) && len(roles) > 0 && stringz.ContainsAny(entityRoles, roles...) {
			err = linkCollection.AddLinks(tx, id, entityId)
		} else {
			err = linkCollection.RemoveLinks(tx, id, entityId)
		}

		holder.SetError(err)
		if holder.HasError() {
			return
		}
	}
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
		return nil, validation.NewFieldError("invalid semantic", FieldSemantic, semantic)
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
