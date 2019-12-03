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
	"github.com/netfoundry/ziti-foundation/storage/ast"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"go.etcd.io/bbolt"
)

const (
	FieldIdentityType             = "type"
	FieldIdentityAppwans          = "appwans"
	FieldIdentityApiSessions      = "apiSessions"
	FieldIdentityHostableServices = "hostableServices"
	FieldIdentityIsDefaultAdmin   = "isDefaultAdmin"
	FieldIdentityIsAdmin          = "isAdmin"
	FieldIdentityEnrollments      = "enrollments"
	FieldIdentityAuthenticators   = "authenticators"
)

type Identity struct {
	BaseEdgeEntityImpl
	Name           string
	IdentityTypeId string
	IsDefaultAdmin bool
	IsAdmin        bool
	Enrollments    []string
	Authenticators []string
}

var identityFieldMappings = map[string]string{FieldIdentityType: "identityTypeId"}

func (entity *Identity) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
	entity.IdentityTypeId = bucket.GetStringWithDefault(FieldIdentityType, "")
	entity.IsDefaultAdmin = bucket.GetBoolWithDefault(FieldIdentityIsDefaultAdmin, false)
	entity.IsAdmin = bucket.GetBoolWithDefault(FieldIdentityIsAdmin, false)
	entity.Authenticators = bucket.GetStringList(FieldIdentityAuthenticators)
	entity.Enrollments = bucket.GetStringList(FieldIdentityEnrollments)
}

func (entity *Identity) SetValues(ctx *boltz.PersistContext) {
	ctx.WithFieldOverrides(identityFieldMappings)

	entity.SetBaseValues(ctx)
	ctx.SetString(FieldName, entity.Name)
	ctx.SetBool(FieldIdentityIsDefaultAdmin, entity.IsDefaultAdmin)
	ctx.SetBool(FieldIdentityIsAdmin, entity.IsAdmin)
	ctx.SetString(FieldIdentityType, entity.IdentityTypeId)
	ctx.SetLinkedIds(FieldIdentityEnrollments, entity.Enrollments)
	ctx.SetLinkedIds(FieldIdentityAuthenticators, entity.Authenticators)
}

func (entity *Identity) GetEntityType() string {
	return EntityTypeIdentities
}

type IdentityStore interface {
	Store
	LoadOneById(tx *bbolt.Tx, id string) (*Identity, error)
	LoadOneByName(tx *bbolt.Tx, id string) (*Identity, error)
	LoadOneByQuery(tx *bbolt.Tx, query string) (*Identity, error)
}

func newIdentityStore(stores *stores) *identityStoreImpl {
	store := &identityStoreImpl{
		baseStore: newBaseStore(stores, EntityTypeIdentities),
	}
	store.InitImpl(store)
	return store
}

type identityStoreImpl struct {
	*baseStore

	indexName boltz.ReadIndex

	symbolApiSessions      boltz.EntitySetSymbol
	symbolAppwans          boltz.EntitySymbol
	symbolEnrollments      boltz.EntitySetSymbol
	symbolAuthenticators   boltz.EntitySetSymbol
	symbolHostableServices boltz.EntitySymbol
	symbolIdentityTypeId   boltz.EntitySymbol
}

func (store *identityStoreImpl) NewStoreEntity() boltz.BaseEntity {
	return &Identity{}
}

func (store *identityStoreImpl) initializeLocal() {
	store.addBaseFields()

	store.indexName = store.addUniqueNameField()
	store.symbolApiSessions = store.AddFkSetSymbol(FieldIdentityApiSessions, store.stores.apiSession)
	store.symbolAppwans = store.AddFkSetSymbol(FieldIdentityAppwans, store.stores.appwan)
	store.symbolHostableServices = store.AddFkSetSymbol(FieldIdentityHostableServices, store.stores.edgeService)

	store.symbolEnrollments = store.AddFkSetSymbol(FieldIdentityEnrollments, store.stores.enrollment)
	store.symbolAuthenticators = store.AddFkSetSymbol(FieldIdentityAuthenticators, store.stores.authenticator)

	store.symbolIdentityTypeId = store.AddFkSymbol(FieldIdentityType, store.stores.identityType)

	store.AddSymbol(FieldIdentityIsAdmin, ast.NodeTypeBool)
	store.AddSymbol(FieldIdentityIsDefaultAdmin, ast.NodeTypeBool)
}

func (store *identityStoreImpl) initializeLinked() {
	store.AddLinkCollection(store.symbolAppwans, store.stores.appwan.symbolIdentities)
	store.AddLinkCollection(store.symbolAuthenticators, store.stores.authenticator.symbolIdentityId)
	store.AddLinkCollection(store.symbolEnrollments, store.stores.enrollment.symbolIdentityId)
}

func (store *identityStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*Identity, error) {
	entity := &Identity{}
	if found, err := store.BaseLoadOneById(tx, id, entity); !found || err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *identityStoreImpl) LoadOneByName(tx *bbolt.Tx, name string) (*Identity, error) {
	id := store.indexName.Read(tx, []byte(name))
	if id != nil {
		return store.LoadOneById(tx, string(id))
	}
	return nil, nil
}

func (store *identityStoreImpl) LoadOneByQuery(tx *bbolt.Tx, query string) (*Identity, error) {
	entity := &Identity{}
	if found, err := store.BaseLoadOneByQuery(tx, query, entity); !found || err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *identityStoreImpl) DeleteById(ctx boltz.MutateContext, id string) error {
	for _, apiSessionId := range store.GetRelatedEntitiesIdList(ctx.Tx(), id, FieldIdentityApiSessions) {
		if err := store.stores.apiSession.DeleteById(ctx, apiSessionId); err != nil {
			return err
		}
	}

	for _, enrollmentId := range store.GetRelatedEntitiesIdList(ctx.Tx(), id, FieldIdentityEnrollments) {
		if err := store.stores.enrollment.DeleteById(ctx, enrollmentId); err != nil {
			return err
		}
	}

	for _, authenticatorId := range store.GetRelatedEntitiesIdList(ctx.Tx(), id, FieldIdentityAuthenticators) {
		if err := store.stores.authenticator.DeleteById(ctx, authenticatorId); err != nil {
			return err
		}
	}

	return store.baseStore.DeleteById(ctx, id)
}
