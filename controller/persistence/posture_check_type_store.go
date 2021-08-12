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
	"github.com/openziti/foundation/storage/boltz"
	"go.etcd.io/bbolt"
)

const (
	FieldPostureCheckTypeOperatingSystems = "operatingSystems"
)

type PostureCheckOs struct {
	boltz.BaseExtEntity
	Name             string
	OperatingSystems []OperatingSystem
}

func (entity *PostureCheckOs) GetName() string {
	return entity.Name
}

func (entity *PostureCheckOs) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)

	osBucket := bucket.GetOrCreateBucket(FieldPostureCheckTypeOperatingSystems)
	cursor := osBucket.Cursor()

	for key, _ := cursor.First(); key != nil; key, _ = cursor.Next() {
		curOs := osBucket.GetBucket(string(key))

		if curOs == nil {
			continue
		}

		newOsMatch := OperatingSystem{
			OsType: curOs.GetStringOrError(FieldPostureCheckOsType),
		}

		for _, osVersion := range curOs.GetStringList(FieldPostureCheckOsVersions) {
			newOsMatch.OsVersions = append(newOsMatch.OsVersions, osVersion)
		}
		entity.OperatingSystems = append(entity.OperatingSystems, newOsMatch)
	}
}

func (entity *PostureCheckOs) SetValues(ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldName, entity.Name)

	osMap := map[string]OperatingSystem{}

	for _, os := range entity.OperatingSystems {
		osMap[os.OsType] = os
	}

	osBucket := ctx.Bucket.GetOrCreateBucket(FieldPostureCheckTypeOperatingSystems)
	cursor := osBucket.Cursor()

	for key, _ := cursor.First(); key != nil; key, _ = cursor.Next() {
		if _, found := osMap[string(key)]; !found {
			_ = osBucket.Delete(key)
		}
	}

	for _, os := range entity.OperatingSystems {
		existing := osBucket.GetOrCreateBucket(os.OsType)
		existing.SetString(FieldPostureCheckOsType, os.OsType, ctx.FieldChecker)
		existing.SetStringList(FieldPostureCheckOsVersions, os.OsVersions, ctx.FieldChecker)
	}
}

func (entity *PostureCheckOs) GetEntityType() string {
	return EntityTypePostureCheckTypes
}

type PostureCheckTypeStore interface {
	NameIndexedStore
	LoadOneById(tx *bbolt.Tx, id string) (*PostureCheckOs, error)
	LoadOneByName(tx *bbolt.Tx, id string) (*PostureCheckOs, error)
}

func newPostureCheckTypeStore(stores *stores) *postureCheckTypeStoreImpl {
	store := &postureCheckTypeStoreImpl{
		baseStore: newBaseStore(stores, EntityTypePostureCheckTypes),
	}
	store.InitImpl(store)
	return store
}

type postureCheckTypeStoreImpl struct {
	*baseStore
	indexName boltz.ReadIndex
}

func (store *postureCheckTypeStoreImpl) NewStoreEntity() boltz.Entity {
	return &PostureCheckOs{}
}

func (store *postureCheckTypeStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()
	store.indexName = store.addUniqueNameField()
}

func (store *postureCheckTypeStoreImpl) initializeLinked() {
	// no links
}

func (store *postureCheckTypeStoreImpl) GetNameIndex() boltz.ReadIndex {
	return store.indexName
}

func (store *postureCheckTypeStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*PostureCheckOs, error) {
	entity := &PostureCheckOs{}
	if err := store.baseLoadOneById(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *postureCheckTypeStoreImpl) LoadOneByName(tx *bbolt.Tx, name string) (*PostureCheckOs, error) {
	id := store.indexName.Read(tx, []byte(name))
	if id != nil {
		return store.LoadOneById(tx, string(id))
	}
	return nil, nil
}

func (store *postureCheckTypeStoreImpl) LoadOneByQuery(tx *bbolt.Tx, query string) (*PostureCheckOs, error) {
	entity := &PostureCheckOs{}
	if found, err := store.BaseLoadOneByQuery(tx, query, entity); !found || err != nil {
		return nil, err
	}
	return entity, nil
}
