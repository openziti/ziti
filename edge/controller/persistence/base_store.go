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
	"github.com/netfoundry/ziti-edge/edge/controller/util"
	"github.com/netfoundry/ziti-fabric/fabric/controller/network"
	"github.com/netfoundry/ziti-foundation/storage/ast"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/netfoundry/ziti-foundation/util/errorz"
	"go.etcd.io/bbolt"
)

type DbProvider interface {
	GetDb() boltz.Db
	GetServiceStore() network.ServiceStore
	GetServiceCache() network.Cache
	GetRouterStore() network.RouterStore
}

type Store interface {
	boltz.CrudStore

	initializeLocal()
	initializeLinked()
	initializeIndexes(tx *bbolt.Tx, errorHolder errorz.ErrorHolder)
}

func newBaseStore(stores *stores, entityType string) *baseStore {
	return &baseStore{
		stores: stores,
		BaseStore: boltz.NewBaseStore(nil, entityType, func(id string) error {
			return util.RecordNotFoundError{}
		}, boltz.RootBucket),
	}
}

func newChildBaseStore(stores *stores, parent boltz.CrudStore, entityType string) *baseStore {
	return &baseStore{
		stores: stores,
		BaseStore: boltz.NewBaseStore(parent, entityType, func(id string) error {
			return util.RecordNotFoundError{}
		}, EdgeBucket),
	}
}

type baseStore struct {
	stores *stores
	*boltz.BaseStore
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

func (store *baseStore) initializeIndexes(tx *bbolt.Tx, errorHolder errorz.ErrorHolder) {
	store.InitializeIndexes(tx, errorHolder)
}
