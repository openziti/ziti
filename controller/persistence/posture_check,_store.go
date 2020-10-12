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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/storage/ast"
	"github.com/openziti/foundation/storage/boltz"
	"go.etcd.io/bbolt"
)

const (
	//Fields
	FieldPostureCheckTypeId      = "typeId"
	FieldPostureCheckVersion     = "version"
	FieldPostureCheckDescription = "description"
)

var postureCheckSubTypeMap = map[string]newPostureCheckSubType{
	"OS":      newPostureCheckOperatingSystem,
	"DOMAIN":  newPostureCheckWindowsDomain,
	"PROCESS": newPostureCheckProcess,
	"MAC":     newPostureCheckMacAddresses,
}

type newPostureCheckSubType func() PostureCheckSubType

type PostureCheckSubType interface {
	LoadValues(store boltz.CrudStore, bucket *boltz.TypedBucket)
	SetValues(ctx *boltz.PersistContext, bucket *boltz.TypedBucket)
}

func newPostureCheck(typeId string) PostureCheckSubType {
	if newChild, found := postureCheckSubTypeMap[typeId]; found {
		return newChild()
	}
	return nil
}

type PostureCheck struct {
	boltz.BaseExtEntity
	Name        string
	TypeId      string
	Description string
	Version     int64
	SubType     PostureCheckSubType
}

func (entity *PostureCheck) GetName() string {
	return entity.Name
}

func (entity *PostureCheck) LoadValues(store boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
	entity.TypeId = bucket.GetStringOrError(FieldPostureCheckTypeId)
	entity.Description = bucket.GetStringOrError(FieldPostureCheckDescription)
	entity.Version = bucket.GetInt64WithDefault(FieldPostureCheckVersion, 0)

	entity.SubType = newPostureCheck(entity.TypeId)
	if entity.SubType == nil {
		pfxlog.Logger().Panicf("cannot load unsupported posture check type [%v]", entity.TypeId)
	}

	childBucket := bucket.GetOrCreateBucket(entity.TypeId)

	entity.SubType.LoadValues(store, childBucket)
}

func (entity *PostureCheck) SetValues(ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldName, entity.Name)
	ctx.SetString(FieldPostureCheckTypeId, entity.TypeId)
	ctx.SetString(FieldPostureCheckDescription, entity.Description)
	ctx.SetInt64(FieldPostureCheckVersion, entity.Version)

	childBucket := ctx.Bucket.GetOrCreateBucket(entity.TypeId)

	entity.SubType.SetValues(ctx, childBucket)

}

func (entity *PostureCheck) GetEntityType() string {
	return EntityTypePostureChecks
}

type PostureCheckStore interface {
	Store
	LoadOneById(tx *bbolt.Tx, id string) (*PostureCheck, error)
	LoadOneByName(tx *bbolt.Tx, id string) (*PostureCheck, error)
	LoadOneByQuery(tx *bbolt.Tx, query string) (*PostureCheck, error)
}

func newPostureCheckStore(stores *stores) *postureCheckStoreImpl {
	store := &postureCheckStoreImpl{
		baseStore: newBaseStore(stores, EntityTypePostureChecks),
	}
	store.InitImpl(store)
	return store
}

type postureCheckStoreImpl struct {
	*baseStore
	indexName boltz.ReadIndex
}

func (store *postureCheckStoreImpl) NewStoreEntity() boltz.Entity {
	return &PostureCheck{}
}

func (store *postureCheckStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()
	store.indexName = store.addUniqueNameField()
	store.AddSymbol(FieldPostureCheckDescription, ast.NodeTypeString)
}

func (store *postureCheckStoreImpl) initializeLinked() {
}

func (store *postureCheckStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*PostureCheck, error) {
	entity := &PostureCheck{}
	if err := store.baseLoadOneById(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *postureCheckStoreImpl) LoadOneByName(tx *bbolt.Tx, name string) (*PostureCheck, error) {
	id := store.indexName.Read(tx, []byte(name))
	if id != nil {
		return store.LoadOneById(tx, string(id))
	}
	return nil, nil
}

func (store *postureCheckStoreImpl) LoadOneByQuery(tx *bbolt.Tx, query string) (*PostureCheck, error) {
	entity := &PostureCheck{}
	if found, err := store.BaseLoadOneByQuery(tx, query, entity); !found || err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *postureCheckStoreImpl) DeleteById(ctx boltz.MutateContext, id string) error {
	return store.baseStore.DeleteById(ctx, id)
}

func (store *postureCheckStoreImpl) Update(ctx boltz.MutateContext, entity boltz.Entity, checker boltz.FieldChecker) error {
	return store.baseStore.Update(ctx, entity, checker)
}
