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
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"go.etcd.io/bbolt"
)

type GeoRegion struct {
	BaseEdgeEntityImpl
	Name string
}

func (entity *GeoRegion) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
}

func (entity *GeoRegion) SetValues(ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldName, entity.Name)
}

func (entity *GeoRegion) GetEntityType() string {
	return EntityTypeGeoRegions
}

type GeoRegionStore interface {
	Store
	LoadOneById(tx *bbolt.Tx, id string) (*GeoRegion, error)
	LoadOneByName(tx *bbolt.Tx, id string) (*GeoRegion, error)
}

func newGeoRegionStore(stores *stores) *geoRegionStoreImpl {
	store := &geoRegionStoreImpl{
		baseStore: newBaseStore(stores, EntityTypeGeoRegions),
	}
	store.InitImpl(store)
	return store
}

type geoRegionStoreImpl struct {
	*baseStore
	indexName boltz.ReadIndex
}

func (store *geoRegionStoreImpl) NewStoreEntity() boltz.BaseEntity {
	return &GeoRegion{}
}

func (store *geoRegionStoreImpl) initializeLocal() {
	store.addBaseFields()
	store.indexName = store.addUniqueNameField()
}

func (store *geoRegionStoreImpl) initializeLinked() {
	// no links
}

func (store *geoRegionStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*GeoRegion, error) {
	entity := &GeoRegion{}
	if err := store.baseLoadOneById(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *geoRegionStoreImpl) LoadOneByName(tx *bbolt.Tx, name string) (*GeoRegion, error) {
	id := store.indexName.Read(tx, []byte(name))
	if id != nil {
		return store.LoadOneById(tx, string(id))
	}
	return nil, nil
}

func (store *geoRegionStoreImpl) LoadOneByQuery(tx *bbolt.Tx, query string) (*GeoRegion, error) {
	entity := &GeoRegion{}
	if found, err := store.BaseLoadOneByQuery(tx, query, entity); !found || err != nil {
		return nil, err
	}
	return entity, nil
}
