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
	"github.com/netfoundry/ziti-foundation/storage/ast"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"go.etcd.io/bbolt"
)

const (
	FieldApiSessionIdentity    = "identity"
	FieldApiSessionToken       = "token"
	FieldApiSessionConfigTypes = "configTypes"
)

type ApiSession struct {
	BaseEdgeEntityImpl
	IdentityId  string
	Token       string
	ConfigTypes []string
}

func NewApiSession(identityId string) *ApiSession {
	return &ApiSession{
		BaseEdgeEntityImpl: BaseEdgeEntityImpl{Id: uuid.New().String()},
		IdentityId:         identityId,
		Token:              uuid.New().String(),
	}
}

func (entity *ApiSession) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.IdentityId = bucket.GetStringOrError(FieldApiSessionIdentity)
	entity.Token = bucket.GetStringOrError(FieldApiSessionToken)
	entity.ConfigTypes = bucket.GetStringList(FieldApiSessionConfigTypes)
}

func (entity *ApiSession) SetValues(ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldApiSessionIdentity, entity.IdentityId)
	ctx.SetString(FieldApiSessionToken, entity.Token)
	ctx.SetStringList(FieldApiSessionConfigTypes, entity.ConfigTypes)
}

func (entity *ApiSession) GetEntityType() string {
	return EntityTypeApiSessions
}

type ApiSessionStore interface {
	Store
	LoadOneById(tx *bbolt.Tx, id string) (*ApiSession, error)
	LoadOneByToken(tx *bbolt.Tx, token string) (*ApiSession, error)
	LoadOneByQuery(tx *bbolt.Tx, query string) (*ApiSession, error)
	GetTokenIndex() boltz.ReadIndex
	MarkActivity(tx *bbolt.Tx, tokens []string) error
}

func newApiSessionStore(stores *stores) *apiSessionStoreImpl {
	store := &apiSessionStoreImpl{
		baseStore: newBaseStore(stores, EntityTypeApiSessions),
	}
	store.InitImpl(store)
	return store
}

type apiSessionStoreImpl struct {
	*baseStore

	indexToken     boltz.ReadIndex
	symbolIdentity boltz.EntitySymbol
	symbolSessions boltz.EntitySetSymbol
}

func (store *apiSessionStoreImpl) NewStoreEntity() boltz.BaseEntity {
	return &ApiSession{}
}

func (store *apiSessionStoreImpl) GetTokenIndex() boltz.ReadIndex {
	return store.indexToken
}

func (store *apiSessionStoreImpl) initializeLocal() {
	store.addBaseFields()
	symbolToken := store.AddSymbol(FieldApiSessionToken, ast.NodeTypeString)
	store.indexToken = store.AddUniqueIndex(symbolToken)
	store.symbolIdentity = store.AddFkSymbol(FieldApiSessionIdentity, store.stores.identity)
	store.symbolSessions = store.AddFkSetSymbol(EntityTypeSessions, store.stores.session)
}

func (store *apiSessionStoreImpl) initializeLinked() {
	store.AddFkIndex(store.symbolIdentity, store.stores.identity.symbolApiSessions)
}

func (store *apiSessionStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*ApiSession, error) {
	entity := &ApiSession{}
	if err := store.baseLoadOneById(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *apiSessionStoreImpl) LoadOneByToken(tx *bbolt.Tx, token string) (*ApiSession, error) {
	id := store.indexToken.Read(tx, []byte(token))
	if id != nil {
		return store.LoadOneById(tx, string(id))
	}
	return nil, nil
}

func (store *apiSessionStoreImpl) LoadOneByQuery(tx *bbolt.Tx, query string) (*ApiSession, error) {
	entity := &ApiSession{}
	if found, err := store.BaseLoadOneByQuery(tx, query, entity); !found || err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *apiSessionStoreImpl) DeleteById(ctx boltz.MutateContext, id string) error {
	for _, sessionId := range store.GetRelatedEntitiesIdList(ctx.Tx(), id, EntityTypeSessions) {
		if err := store.stores.session.DeleteById(ctx, sessionId); err != nil {
			return err
		}
	}
	return store.baseStore.DeleteById(ctx, id)
}

func (store *apiSessionStoreImpl) MarkActivity(tx *bbolt.Tx, tokens []string) error {
	mutCtx := boltz.NewMutateContext(tx)
	for _, token := range tokens {
		apiSession, err := store.LoadOneByToken(tx, token)
		if err != nil {
			return err
		}
		if err = store.Update(mutCtx, apiSession, UpdateTimeOnlyFieldChecker{}); err != nil {
			return err
		}
	}
	return nil
}
