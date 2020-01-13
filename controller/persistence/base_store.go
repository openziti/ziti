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
	"github.com/netfoundry/ziti-edge/controller/util"
	"github.com/netfoundry/ziti-fabric/controller/network"
	"github.com/netfoundry/ziti-foundation/storage/ast"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/netfoundry/ziti-foundation/util/errorz"
	"github.com/netfoundry/ziti-foundation/util/stringz"
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

func (store *baseStore) getEntityIdsForRoleSet(tx *bbolt.Tx, roleSet []string, index boltz.SetReadIndex, targetStore NameIndexedStore) ([]string, error) {
	entityStore := index.GetSymbol().GetStore()
	roles, ids, err := splitRolesAndIds(roleSet)
	if err != nil {
		return nil, err
	}
	ConvertNamesToIds(targetStore, tx, ids)
	if stringz.Contains(roles, "all") {
		ids, _, err := entityStore.QueryIds(tx, "true")
		if err != nil {
			return nil, err
		}
		return ids, nil
	}
	roleIds := entityStore.FindMatching(tx, index, roles)
	for _, id := range ids {
		if entityStore.IsEntityPresent(tx, id) {
			roleIds = append(roleIds, id)
		}
	}
	return roleIds, nil
}

func ConvertNamesToIds(store NameIndexedStore, tx *bbolt.Tx, ids []string) {
	nameIndex := store.GetNameIndex()
	for idx, val := range ids {
		id := nameIndex.Read(tx, []byte(val))
		if id != nil {
			ids[idx] = string(id)
		}
	}
}

func UpdateRelatedRoles(store NameIndexedStore, tx *bbolt.Tx, entityId string, roleSymbol boltz.EntitySetSymbol, linkCollection boltz.LinkCollection, new []boltz.FieldTypeAndValue, holder errorz.ErrorHolder) {
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
		ConvertNamesToIds(store, tx, ids)
		if stringz.Contains(ids, entityId) || stringz.Contains(roles, "all") || (len(roles) > 0 && stringz.ContainsAll(entityRoles, roles...)) {
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
