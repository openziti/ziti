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
	"fmt"
	"github.com/openziti/storage/boltz"
)

const (
	TransitRouterPath             = "transitRouter"
	FieldTransitRouterIsVerified  = "isVerified"
	FieldTransitRouterEnrollments = "enrollments"
)

type TransitRouter struct {
	Router
	IsVerified            bool     `json:"isVerified"`
	Enrollments           []string `json:"enrollments"`
	IsBase                bool     `json:"-"`
	UnverifiedCertPem     *string  `json:"unverifiedCertPem"`
	UnverifiedFingerprint *string  `json:"unverifiedFingerprint"`
}

func (entity *TransitRouter) GetName() string {
	return entity.Name
}

var _ TransitRouterStore = (*transitRouterStoreImpl)(nil)

type TransitRouterStore interface {
	NameIndexed
	Store[*TransitRouter]
}

func newTransitRouterStore(stores *stores) *transitRouterStoreImpl {
	parentMapper := func(entity boltz.Entity) boltz.Entity {
		if transitRouter, ok := entity.(*TransitRouter); ok {
			return &transitRouter.Router
		}
		return entity
	}

	store := &transitRouterStoreImpl{}
	store.baseStore = newChildBaseStore[*TransitRouter](stores, parentMapper, store, stores.router, TransitRouterPath)
	store.Extended()

	store.InitImpl(store)

	return store
}

type transitRouterStoreImpl struct {
	*baseStore[*TransitRouter]
	indexName         boltz.ReadIndex
	symbolEnrollments boltz.EntitySetSymbol
}

func (store *transitRouterStoreImpl) HandleUpdate(ctx boltz.MutateContext, entity *Router, checker boltz.FieldChecker) (bool, error) {
	er, found, err := store.FindById(ctx.Tx(), entity.Id)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}

	er.Router = *entity
	return true, store.Update(ctx, er, checker)
}

func (store *transitRouterStoreImpl) HandleDelete(ctx boltz.MutateContext, entity *Router) error {
	return store.cleanupEnrollments(ctx, entity.Id)
}

func (store *transitRouterStoreImpl) GetStore() boltz.Store {
	return store
}

func (store *transitRouterStoreImpl) NewEntity() *TransitRouter {
	return &TransitRouter{}
}

func (store *transitRouterStoreImpl) initializeLocal() {
	store.GetParentStore().GrantSymbols(store)
	store.indexName = store.GetParentStore().(RouterStore).GetNameIndex()
	store.symbolEnrollments = store.AddFkSetSymbol(FieldTransitRouterEnrollments, store.stores.enrollment)
}

func (store *transitRouterStoreImpl) initializeLinked() {
}

func (store *transitRouterStoreImpl) GetNameIndex() boltz.ReadIndex {
	return store.indexName
}

func (store *transitRouterStoreImpl) FillEntity(entity *TransitRouter, bucket *boltz.TypedBucket) {
	store.stores.router.FillEntity(&entity.Router, store.getParentBucket(entity, bucket))

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

func (store *transitRouterStoreImpl) PersistEntity(entity *TransitRouter, ctx *boltz.PersistContext) {
	store.stores.router.PersistEntity(&entity.Router, ctx.GetParentContext())
	if ctx.Bucket != nil {
		ctx.SetBool(FieldTransitRouterIsVerified, entity.IsVerified)
		ctx.SetStringP(FieldEdgeRouterUnverifiedFingerprint, entity.UnverifiedFingerprint)
		ctx.SetStringP(FieldEdgeRouterUnverifiedCertPEM, entity.UnverifiedCertPem)
	}
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
