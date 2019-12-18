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
	"github.com/google/uuid"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

const (
	FieldClusterEdgeRouters = "edgeRouters"
	FieldClusterServices    = "services"
)

type Cluster struct {
	BaseEdgeEntityImpl
	Name string
}

func NewCluster(name string) *Cluster {
	return &Cluster{
		BaseEdgeEntityImpl: BaseEdgeEntityImpl{Id: uuid.New().String()},
		Name:               name,
	}
}

func (entity *Cluster) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
}

func (entity *Cluster) SetValues(ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldName, entity.Name)
	// EdgeRouters are managed from edgeRouter
	// Services are managed from services
}

func (entity *Cluster) GetEntityType() string {
	return EntityTypeClusters
}

type ClusterStore interface {
	Store
	LoadOneById(tx *bbolt.Tx, id string) (*Cluster, error)
	LoadOneByName(tx *bbolt.Tx, id string) (*Cluster, error)
	LoadOneByQuery(tx *bbolt.Tx, query string) (*Cluster, error)
}

func newClusterStore(stores *stores) *clusterStoreImpl {
	store := &clusterStoreImpl{
		baseStore: newBaseStore(stores, EntityTypeClusters),
	}
	store.InitImpl(store)
	return store
}

type clusterStoreImpl struct {
	*baseStore

	indexName         boltz.ReadIndex
	symbolEdgeRouters boltz.EntitySetSymbol
	symbolServices    boltz.EntitySetSymbol
}

func (store *clusterStoreImpl) NewStoreEntity() boltz.BaseEntity {
	return &Cluster{}
}

func (store *clusterStoreImpl) initializeLocal() {
	store.addBaseFields()

	store.indexName = store.addUniqueNameField()
	store.symbolEdgeRouters = store.AddFkSetSymbol(FieldClusterEdgeRouters, store.stores.edgeRouter)
	store.symbolServices = store.AddFkSetSymbol(FieldClusterServices, store.stores.edgeService)
}

func (store *clusterStoreImpl) initializeLinked() {
	store.AddLinkCollection(store.symbolServices, store.stores.edgeService.symbolClusters)
}

func (store *clusterStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*Cluster, error) {
	entity := &Cluster{}
	if err := store.baseLoadOneById(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *clusterStoreImpl) LoadOneByName(tx *bbolt.Tx, name string) (*Cluster, error) {
	id := store.indexName.Read(tx, []byte(name))
	if id != nil {
		return store.LoadOneById(tx, string(id))
	}
	return nil, nil
}

func (store *clusterStoreImpl) LoadOneByQuery(tx *bbolt.Tx, query string) (*Cluster, error) {
	entity := &Cluster{}
	if found, err := store.BaseLoadOneByQuery(tx, query, entity); !found || err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *clusterStoreImpl) DeleteById(ctx boltz.MutateContext, id string) error {
	if bucket := store.GetEntityBucket(ctx.Tx(), []byte(id)); bucket != nil {
		if !bucket.IsStringListEmpty(FieldClusterEdgeRouters) {
			return errors.Errorf("cannot delete cluster %v, which has edge routers assigned to it", id)
		}

		if !bucket.IsStringListEmpty(FieldClusterServices) {
			return errors.Errorf("cannot delete cluster %v, which has services assigned to it", id)
		}
	}

	return store.baseStore.DeleteById(ctx, id)
}
