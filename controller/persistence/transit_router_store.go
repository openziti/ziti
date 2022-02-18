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
	"fmt"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/foundation/storage/boltz"
	"go.etcd.io/bbolt"
)

const (
	TransitRouterPath             = "transitRouter"
	FieldTransitRouterIsVerified  = "isVerified"
	FieldTransitRouterEnrollments = "enrollments"
)

type TransitRouter struct {
	db.Router
	IsVerified            bool
	Enrollments           []string
	IsBase                bool
	UnverifiedCertPem     *string
	UnverifiedFingerprint *string
}

func (entity *TransitRouter) LoadValues(store boltz.CrudStore, bucket *boltz.TypedBucket) {
	_, err := store.GetParentStore().BaseLoadOneById(bucket.Tx(), entity.Id, &entity.Router)
	bucket.SetError(err)

	if bucket.Bucket == nil {
		entity.IsVerified = true
		entity.IsBase = true
		return
	}

	entity.IsVerified = bucket.GetBoolWithDefault(FieldTransitRouterIsVerified, false)
	entity.Enrollments = bucket.GetStringList(FieldTransitRouterEnrollments)
	entity.UnverifiedFingerprint = bucket.GetString(FieldEdgeRouterUnverifiedFingerprint)
	entity.UnverifiedCertPem = bucket.GetString(FieldEdgeRouterUnverifiedCertPEM)
}

func (entity *TransitRouter) SetValues(ctx *boltz.PersistContext) {
	entity.Router.SetValues(ctx.GetParentContext())
	if ctx.Bucket != nil {
		ctx.SetBool(FieldTransitRouterIsVerified, entity.IsVerified)
		ctx.SetStringP(FieldEdgeRouterUnverifiedFingerprint, entity.UnverifiedFingerprint)
		ctx.SetStringP(FieldEdgeRouterUnverifiedCertPEM, entity.UnverifiedCertPem)
	}
}

func (entity *TransitRouter) GetEntityType() string {
	return db.EntityTypeRouters
}

func (entity *TransitRouter) GetName() string {
	return entity.Name
}

type TransitRouterStore interface {
	NameIndexedStore
	LoadOneById(tx *bbolt.Tx, id string) (*TransitRouter, error)
	LoadOneByName(tx *bbolt.Tx, id string) (*TransitRouter, error)
}

func newTransitRouterStore(stores *stores) *transitRouterStoreImpl {
	parentMapper := func(entity boltz.Entity) boltz.Entity {
		if transitRouter, ok := entity.(*TransitRouter); ok {
			return &transitRouter.Router
		}
		return entity
	}

	store := &transitRouterStoreImpl{}
	stores.Router.AddDeleteHandler(store.cleanupEnrollments) // do cleanup first
	store.baseStore = newExtendedBaseStore(stores, stores.Router, parentMapper, TransitRouterPath)
	store.InitImpl(store)

	return store
}

type transitRouterStoreImpl struct {
	*baseStore
	indexName         boltz.ReadIndex
	symbolEnrollments boltz.EntitySetSymbol
}

func (store *transitRouterStoreImpl) NewStoreEntity() boltz.Entity {
	return &TransitRouter{}
}

func (store *transitRouterStoreImpl) initializeLocal() {
	store.GetParentStore().GrantSymbols(store)
	store.indexName = store.GetParentStore().(db.RouterStore).GetNameIndex()
	store.symbolEnrollments = store.AddFkSetSymbol(FieldTransitRouterEnrollments, store.stores.enrollment)
}

func (store *transitRouterStoreImpl) initializeLinked() {
}

func (store *transitRouterStoreImpl) GetNameIndex() boltz.ReadIndex {
	return store.indexName
}

func (store *transitRouterStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*TransitRouter, error) {
	entity := &TransitRouter{}
	if err := store.baseLoadOneById(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *transitRouterStoreImpl) LoadOneByName(tx *bbolt.Tx, name string) (*TransitRouter, error) {
	id := store.indexName.Read(tx, []byte(name))
	if id != nil {
		return store.LoadOneById(tx, string(id))
	}
	return nil, nil
}

func (store *transitRouterStoreImpl) LoadOneByQuery(tx *bbolt.Tx, query string) (*TransitRouter, error) {
	entity := &TransitRouter{}
	if found, err := store.BaseLoadOneByQuery(tx, query, entity); !found || err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *transitRouterStoreImpl) cleanupEnrollments(ctx boltz.MutateContext, id string) error {
	if entity, _ := store.LoadOneById(ctx.Tx(), id); entity != nil {
		// Remove outstanding enrollments
		if err := store.stores.enrollment.DeleteWhere(ctx, fmt.Sprintf(`transitRouter="%s"`, entity.Id)); err != nil {
			return err
		}
	}
	return nil
}
