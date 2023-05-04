/*
	Copyright NetFoundry Inc.

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

package db

import (
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	"go.etcd.io/bbolt"
)

const (
	EntityTypeRouters      = "routers"
	FieldRouterFingerprint = "fingerprint"
	FieldRouterCost        = "cost"
	FieldRouterNoTraversal = "noTraversal"
	FieldRouterDisabled    = "disabled"
)

type Router struct {
	boltz.BaseExtEntity
	Name        string  `json:"name"`
	Fingerprint *string `json:"fingerprint"`
	Cost        uint16  `json:"cost"`
	NoTraversal bool    `json:"noTraversal"`
	Disabled    bool    `json:"disabled"`
}

func (entity *Router) GetEntityType() string {
	return EntityTypeRouters
}

type RouterStore interface {
	boltz.EntityStore[*Router]
	boltz.EntityStrategy[*Router]
	GetNameIndex() boltz.ReadIndex
	FindByName(tx *bbolt.Tx, id string) (*Router, error)
}

func newRouterStore(stores *stores) *routerStoreImpl {
	store := &routerStoreImpl{}
	store.baseStore = baseStore[*Router]{
		stores:    stores,
		BaseStore: boltz.NewBaseStore(NewStoreDefinition[*Router](store)),
	}
	store.InitImpl(store)
	return store
}

type routerStoreImpl struct {
	baseStore[*Router]
	indexName         boltz.ReadIndex
	terminatorsSymbol boltz.EntitySetSymbol
}

func (store *routerStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()

	symbolName := store.AddSymbol(FieldName, ast.NodeTypeString)
	store.indexName = store.AddUniqueIndex(symbolName)

	store.AddSymbol(FieldRouterFingerprint, ast.NodeTypeString)
	store.terminatorsSymbol = store.AddFkSetSymbol(EntityTypeTerminators, store.stores.terminator)
	store.AddSymbol(FieldRouterCost, ast.NodeTypeInt64)
	store.AddSymbol(FieldRouterNoTraversal, ast.NodeTypeBool)
	store.AddSymbol(FieldRouterDisabled, ast.NodeTypeBool)
}

func (store *routerStoreImpl) initializeLinked() {
}

func (self *routerStoreImpl) NewEntity() *Router {
	return &Router{}
}

func (self *routerStoreImpl) FillEntity(entity *Router, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
	entity.Fingerprint = bucket.GetString(FieldRouterFingerprint)
	entity.Cost = uint16(bucket.GetInt32WithDefault(FieldRouterCost, 0))
	entity.NoTraversal = bucket.GetBoolWithDefault(FieldRouterNoTraversal, false)
	entity.Disabled = bucket.GetBoolWithDefault(FieldRouterDisabled, false)
}

func (self *routerStoreImpl) PersistEntity(entity *Router, ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldName, entity.Name)
	ctx.SetStringP(FieldRouterFingerprint, entity.Fingerprint)
	ctx.SetInt32(FieldRouterCost, int32(entity.Cost))
	ctx.SetBool(FieldRouterNoTraversal, entity.NoTraversal)
	ctx.SetBool(FieldRouterDisabled, entity.Disabled)
}

func (store *routerStoreImpl) GetNameIndex() boltz.ReadIndex {
	return store.indexName
}

func (store *routerStoreImpl) FindByName(tx *bbolt.Tx, name string) (*Router, error) {
	id := store.indexName.Read(tx, []byte(name))
	if id != nil {
		entity, _, err := store.FindById(tx, string(id))
		return entity, err
	}
	return nil, nil
}

func (store *routerStoreImpl) DeleteById(ctx boltz.MutateContext, id string) error {
	terminatorIds := store.GetRelatedEntitiesIdList(ctx.Tx(), id, EntityTypeTerminators)
	for _, terminatorId := range terminatorIds {
		if err := store.stores.terminator.DeleteById(ctx, terminatorId); err != nil {
			return err
		}
	}
	return store.BaseStore.DeleteById(ctx, id)
}
