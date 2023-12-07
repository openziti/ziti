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
	"github.com/openziti/storage/boltz"
)

const (
	RouterIdentityType  = "Router"
	DefaultIdentityType = "Default"
)

type IdentityType struct {
	boltz.BaseExtEntity
	Name string `json:"name"`
}

func (entity *IdentityType) GetName() string {
	return entity.Name
}

func (entity *IdentityType) GetEntityType() string {
	return EntityTypeIdentityTypes
}

var _ IdentityTypeStore = (*IdentityTypeStoreImpl)(nil)

type IdentityTypeStore interface {
	NameIndexed
	Store[*IdentityType]
}

func newIdentityTypeStore(stores *stores) *IdentityTypeStoreImpl {
	store := &IdentityTypeStoreImpl{}
	store.baseStore = newBaseStore[*IdentityType](stores, store)
	store.InitImpl(store)
	return store
}

type IdentityTypeStoreImpl struct {
	*baseStore[*IdentityType]
	indexName boltz.ReadIndex
}

func (store *IdentityTypeStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()
	store.indexName = store.addUniqueNameField()
}

func (store *IdentityTypeStoreImpl) initializeLinked() {
	// no links
}

func (store *IdentityTypeStoreImpl) GetNameIndex() boltz.ReadIndex {
	return store.indexName
}

func (store *IdentityTypeStoreImpl) NewEntity() *IdentityType {
	return &IdentityType{}
}

func (store *IdentityTypeStoreImpl) FillEntity(entity *IdentityType, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
}

func (store *IdentityTypeStoreImpl) PersistEntity(entity *IdentityType, ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldName, entity.Name)
}
