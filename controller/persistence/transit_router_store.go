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
	"github.com/google/uuid"
	"github.com/netfoundry/ziti-fabric/controller/db"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"go.etcd.io/bbolt"
)

const (
	FieldTransitRouterIsVerified  = "isVerified"
	FieldTransitRouterEnrollments = "enrollments"

	//old
	FieldTransitRouterEnrollmentToken     = "enrollmentToken"
	FieldTransitRouterEnrollmentJwt       = "enrollmentJwt"
	FieldTransitRouterEnrollmentCreatedAt = "enrollmentCreatedAt"
	FieldTransitRouterEnrollmentExpiresAt = "enrollmentExpiresAt"
)

type TransitRouter struct {
	db.Router
	Name        string
	IsVerified  bool
	Enrollments []string
	IsBase      bool
}

func (entity *TransitRouter) GetId() string {
	return entity.Id
}

func (entity *TransitRouter) SetId(id string) {
	entity.Id = id
}

func newTransitRouter(name string, roleAttributes ...string) *TransitRouter {
	return &TransitRouter{
		Router: db.Router{
			BaseExtEntity: boltz.BaseExtEntity{Id: uuid.New().String()},
		},
		Name: name,
	}
}

func (entity *TransitRouter) LoadValues(store boltz.CrudStore, bucket *boltz.TypedBucket) {
	_, err := store.GetParentStore().BaseLoadOneById(bucket.Tx(), entity.Id, &entity.Router)
	bucket.SetError(err)

	if bucket.Bucket == nil {
		entity.IsVerified = true
		entity.IsBase = true
		return
	}
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
	entity.IsVerified = bucket.GetBoolWithDefault(FieldTransitRouterIsVerified, false)
	entity.Enrollments = bucket.GetStringList(FieldTransitRouterEnrollments)
}

func (entity *TransitRouter) SetValues(ctx *boltz.PersistContext) {
	entity.Router.SetValues(ctx.GetParentContext())
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldName, entity.Name)
}

func (entity *TransitRouter) GetEntityType() string {
	return EntityTypeTransitRouters
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
	store := &transitRouterStoreImpl{
		baseStore: newExtendedBaseStore(stores, stores.Router, EntityTypeTransitRouters),
	}
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
	store.AddExtEntitySymbols()
	store.GetParentStore().GrantSymbols(store)

	store.indexName = store.addUniqueNameField()
	store.symbolEnrollments = store.AddFkSetSymbol(FieldTransitRouterEnrollments, store.stores.enrollment)
}

func (store *transitRouterStoreImpl) initializeLinked() {
	store.AddLinkCollection(store.symbolEnrollments, store.stores.enrollment.symbolIdentity)
}

func (store *transitRouterStoreImpl) CleanupExternal(ctx boltz.MutateContext, id string) error {
	entity, err := store.LoadOneById(ctx.Tx(), id)
	if err != nil {
		return err
	}

	//no edge cleanup
	if entity.IsBase {
		return nil
	}

	//edge cleanup
	if err = store.stores.enrollment.DeleteWhere(ctx, fmt.Sprintf(`transitRouter="%s"`, id)); err != nil {
		return nil
	}

	return store.BaseStore.CleanupExternal(ctx, id)
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

func (store *transitRouterStoreImpl) DeleteById(ctx boltz.MutateContext, id string) error {
	if entity, _ := store.LoadOneById(ctx.Tx(), id); entity != nil {
		// Remove outstanding enrollments
		if err := store.stores.enrollment.DeleteWhere(ctx, fmt.Sprintf(`transitRouter="%s"`, entity.Id)); err != nil {
			return err
		}
	}
	return store.BaseStore.DeleteById(ctx, id)
}
