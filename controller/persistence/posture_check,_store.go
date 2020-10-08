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
	"github.com/openziti/foundation/storage/ast"
	"github.com/openziti/foundation/storage/boltz"
	"go.etcd.io/bbolt"
)

const (
	//Fields
	FieldPostureCheckDescription               = "description"
)

type PostureCheck struct {
	boltz.BaseExtEntity
	Name                      string
	Fingerprint               string
	CertPem                   string
	IsVerified                bool
	VerificationToken         string
	IsAutoPostureCheckEnrollmentEnabled bool
	IsOttPostureCheckEnrollmentEnabled  bool
	IsAuthEnabled             bool
	IdentityRoles             []string
	IdentityNameFormat        string
}

func (entity *PostureCheck) GetName() string {
	return entity.Name
}

func (entity *PostureCheck) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)


}

func (entity *PostureCheck) SetValues(ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldName, entity.Name)
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
