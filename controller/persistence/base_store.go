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

package persistence

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/util"
	"github.com/netfoundry/ziti-edge/controller/validation"
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
	GetServiceStore() network.ServiceStore
	GetServiceCache() network.Cache
	GetRouterStore() network.RouterStore
}

type Store interface {
	boltz.CrudStore
	GetSingularEntityType() string
	FindMatchingAnyOf(tx *bbolt.Tx, readIndex boltz.SetReadIndex, values []string) []string
	initializeLocal()
	initializeLinked()
	initializeIndexes(tx *bbolt.Tx, errorHolder errorz.ErrorHolder)
}

func newBaseStore(stores *stores, entityType string) *baseStore {
	singularEntityType := getSingularEntityType(entityType)
	return &baseStore{
		stores: stores,
		BaseStore: boltz.NewBaseStore(nil, entityType, func(id string) error {
			return util.NewNotFoundError(singularEntityType, "id", id)
		}, boltz.RootBucket),
		singularEntityType: singularEntityType,
	}
}

func newChildBaseStore(stores *stores, parent boltz.CrudStore, entityType string) *baseStore {
	singularEntityType := getSingularEntityType(entityType)
	return &baseStore{
		stores: stores,
		BaseStore: boltz.NewBaseStore(parent, entityType, func(id string) error {
			return util.NewNotFoundError(singularEntityType, "id", id)
		}, EdgeBucket),
		singularEntityType: singularEntityType,
	}
}

type baseStore struct {
	stores *stores
	*boltz.BaseStore
	singularEntityType string
}

func (store *baseStore) GetSingularEntityType() string {
	return store.singularEntityType
}

func (store *baseStore) addBaseFields() {
	store.AddIdSymbol(FieldId, ast.NodeTypeString)
	store.AddSymbol(FieldCreatedAt, ast.NodeTypeDatetime)
	store.AddSymbol(FieldUpdatedAt, ast.NodeTypeDatetime)
	store.AddMapSymbol(FieldTags, ast.NodeTypeAnyType, FieldTags)
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

func (store *baseStore) baseLoadOneById(tx *bbolt.Tx, id string, entity boltz.BaseEntity) error {
	found, err := store.BaseLoadOneById(tx, id, entity)
	if err != nil {
		return err
	}
	if !found {
		return util.NewNotFoundError(store.GetSingularEntityType(), "id", id)
	}
	return nil
}

func (store *baseStore) updateEntityNameReferences(bucket *boltz.TypedBucket, rolesSymbol boltz.EntitySetSymbol, entity NamedEdgeEntity, oldName string) {
	// If the entity name is the same as entity ID, bail out. We don't want to remove any references by ID, since
	// those take precedence over named references
	if store.IsEntityPresent(bucket.Tx(), oldName) {
		pfxlog.Logger().Warnf("%v has name %v which is also used as an ID", store.GetSingularEntityType(), oldName)
		return
	}
	oldNameRef := entityRef(oldName)
	newNameRef := entityRef(entity.GetName())
	for _, policyHolderId := range store.GetRelatedEntitiesIdList(bucket.Tx(), entity.GetId(), rolesSymbol.GetStore().GetEntityType()) {
		err := rolesSymbol.Map(bucket.Tx(), []byte(policyHolderId), func(rolesElem string) (s *string, b bool, b2 bool) {
			if rolesElem == oldNameRef {
				return &newNameRef, true, true
			}
			return nil, false, true
		})
		if err != nil {
			bucket.SetError(err)
			return
		}
	}
}

func (store *baseStore) deleteEntityReferences(tx *bbolt.Tx, entity NamedEdgeEntity, rolesSymbol boltz.EntitySetSymbol) error {
	// If the entity name is the same as entity ID, don't remove any of those references as id references take precedence
	checkName := !store.IsEntityPresent(tx, entity.GetName())
	if !checkName {
		pfxlog.Logger().Warnf("%v has name %v which is also used as an ID", store.GetSingularEntityType(), entity.GetName())
	}
	idRef := entityRef(entity.GetId())
	nameRef := entityRef(entity.GetName())

	for _, policyHolderId := range store.GetRelatedEntitiesIdList(tx, entity.GetId(), rolesSymbol.GetStore().GetEntityType()) {
		err := rolesSymbol.Map(tx, []byte(policyHolderId), func(rolesElem string) (s *string, b bool, b2 bool) {
			if rolesElem == idRef || (checkName && rolesElem == nameRef) {
				return nil, true, true
			}
			return nil, false, true
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
	if err := validateAndConvertNamesToIds(tx, targetStore, field, ids); err != nil {
		return nil, err
	}
	var roleIds []string
	if semantic == SemanticAllOf {
		roleIds = store.FindMatching(tx, index, roles)
	} else if semantic == SemanticAnyOf {
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

func validateAndConvertNamesToIds(tx *bbolt.Tx, store NameIndexedStore, field string, ids []string) error {
	nameIndex := store.GetNameIndex()
	var invalid []string
	for idx, val := range ids {
		if !store.IsEntityPresent(tx, val) {
			id := nameIndex.Read(tx, []byte(val))
			if id != nil {
				ids[idx] = string(id)
			} else {
				invalid = append(invalid, val)
			}
		}
	}
	if len(invalid) > 0 {
		return validation.NewFieldError(fmt.Sprintf("no %v found with the given names/ids", store.GetEntityType()), field, invalid)
	}
	return nil
}

func UpdateRelatedRoles(store NameIndexedStore, tx *bbolt.Tx, entityId string, roleSymbol boltz.EntitySetSymbol,
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
		convertNamesToIds(tx, store, ids)
		semantic := SemanticAllOf
		if semanticSymbol != nil {
			if _, semanticValue := semanticSymbol.Eval(tx, []byte(id)); semanticValue != nil {
				semantic = string(semanticValue)
			}
		}

		if stringz.Contains(ids, entityId) || stringz.Contains(roles, "all") {
			err = linkCollection.AddLinks(tx, id, entityId)
		} else if semantic == SemanticAllOf && len(roles) > 0 && stringz.ContainsAll(entityRoles, roles...) {
			err = linkCollection.AddLinks(tx, id, entityId)
		} else if semantic == SemanticAnyOf && len(roles) > 0 && stringz.ContainsAny(entityRoles, roles...) {
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

func convertNamesToIds(tx *bbolt.Tx, store NameIndexedStore, ids []string) {
	nameIndex := store.GetNameIndex()
	for idx, val := range ids {
		id := nameIndex.Read(tx, []byte(val))
		if id != nil {
			ids[idx] = string(id)
		}
	}
}

func getSingularEntityType(entityType string) string {
	if strings.HasSuffix(entityType, "ies") {
		return strings.TrimSuffix(entityType, "ies") + "y"
	}
	return strings.TrimSuffix(entityType, "s")
}

type NameIndexedStore interface {
	Store
	GetNameIndex() boltz.ReadIndex
}

func (*baseStore) FindMatchingAnyOf(tx *bbolt.Tx, readIndex boltz.SetReadIndex, values []string) []string {
	if len(values) == 0 {
		return nil
	}
	var result []string
	if len(values) == 1 {
		readIndex.Read(tx, []byte(values[0]), func(val []byte) {
			result = append(result, string(val))
		})
		return result
	}

	// If there are multiple roles, we want to avoid duplicates
	set := map[string]struct{}{}
	for _, role := range values {
		readIndex.Read(tx, []byte(role), func(val []byte) {
			set[string(val)] = struct{}{}
		})
	}

	for key := range set {
		result = append(result, key)
	}

	return result
}
