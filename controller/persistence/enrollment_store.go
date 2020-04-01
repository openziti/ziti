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
	"github.com/netfoundry/ziti-foundation/storage/ast"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"go.etcd.io/bbolt"
	"time"
)

const (
	FieldEnrollmentToken     = "token"
	FieldEnrollmentMethod    = "method"
	FieldEnrollIdentity      = "identity"
	FieldEnrollEdgeRouter    = "edgeRouter"
	FieldEnrollTransitRouter = "transitRouter"
	FieldEnrollmentExpiresAt = "expiresAt"
	FieldEnrollmentIssuedAt  = "issuedAt"
	FieldEnrollmentCaId      = "caId"
	FieldEnrollmentUsername  = "username"
	FieldEnrollmentJwt       = "jwt"

	MethodEnrollOtt   = "ott"
	MethodEnrollOttCa = "ottca"
	MethodEnrollCa    = "ca"
	MethodEnrollUpdb  = "updb"
)

type Enrollment struct {
	boltz.BaseExtEntity
	Token           string
	Method          string
	IdentityId      *string
	TransitRouterId *string
	EdgeRouterId    *string
	ExpiresAt       *time.Time
	IssuedAt        *time.Time
	CaId            *string
	Username        *string
	Jwt             string
}

var enrollmentFieldMappings = map[string]string{
	FieldEnrollIdentity:      "identityId",
	FieldEnrollEdgeRouter:    "edgeRouterId",
	FieldEnrollTransitRouter: "transitRouterId",
}

func (entity *Enrollment) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.Token = bucket.GetStringWithDefault(FieldEnrollmentToken, "")
	entity.Method = bucket.GetStringWithDefault(FieldEnrollmentMethod, "")
	entity.IdentityId = bucket.GetString(FieldEnrollIdentity)
	entity.EdgeRouterId = bucket.GetString(FieldEnrollEdgeRouter)
	entity.TransitRouterId = bucket.GetString(FieldEnrollTransitRouter)
	entity.ExpiresAt = bucket.GetTime(FieldEnrollmentExpiresAt)
	entity.IssuedAt = bucket.GetTime(FieldEnrollmentIssuedAt)
	entity.CaId = bucket.GetString(FieldEnrollmentCaId)
	entity.Username = bucket.GetString(FieldEnrollmentUsername)
	entity.Jwt = bucket.GetStringOrError(FieldEnrollmentJwt)
}

func (entity *Enrollment) SetValues(ctx *boltz.PersistContext) {
	ctx.WithFieldOverrides(enrollmentFieldMappings)

	ctx.SetString(FieldEnrollmentToken, entity.Token)
	ctx.SetString(FieldEnrollmentMethod, entity.Method)
	ctx.SetTimeP(FieldEnrollmentExpiresAt, entity.ExpiresAt)
	ctx.SetStringP(FieldEnrollIdentity, entity.IdentityId)
	ctx.SetStringP(FieldEnrollEdgeRouter, entity.EdgeRouterId)
	ctx.SetStringP(FieldEnrollTransitRouter, entity.TransitRouterId)
	ctx.SetStringP(FieldEnrollmentCaId, entity.CaId)
	ctx.SetStringP(FieldEnrollmentUsername, entity.Username)
	ctx.SetTimeP(FieldEnrollmentIssuedAt, entity.IssuedAt)
	ctx.SetString(FieldEnrollmentJwt, entity.Jwt)
}

func (entity *Enrollment) GetEntityType() string {
	return EntityTypeEnrollments
}

type EnrollmentStore interface {
	Store
	LoadOneById(tx *bbolt.Tx, id string) (*Enrollment, error)
	LoadOneByToken(tx *bbolt.Tx, token string) (*Enrollment, error)
	LoadOneByQuery(tx *bbolt.Tx, query string) (*Enrollment, error)
}

func newEnrollmentStore(stores *stores) *enrollmentStoreImpl {
	store := &enrollmentStoreImpl{
		baseStore: newBaseStore(stores, EntityTypeEnrollments),
	}
	store.InitImpl(store)

	return store
}

type enrollmentStoreImpl struct {
	*baseStore
	tokenIndex          boltz.ReadIndex
	symbolIdentity      boltz.EntitySymbol
	symbolEdgeRouter    boltz.EntitySymbol
	symbolTransitRouter boltz.EntitySymbol
}

func (store *enrollmentStoreImpl) NewStoreEntity() boltz.Entity {
	return &Enrollment{}
}

func (store *enrollmentStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()

	symbolToken := store.AddSymbol(FieldEnrollmentToken, ast.NodeTypeString)
	store.tokenIndex = store.AddUniqueIndex(symbolToken)
	store.symbolIdentity = store.AddFkSymbol(FieldEnrollIdentity, store.stores.identity)
	store.symbolEdgeRouter = store.AddFkSymbol(FieldEnrollEdgeRouter, store.stores.edgeRouter)
	store.symbolTransitRouter = store.AddFkSymbol(FieldEnrollTransitRouter, store.stores.transitRouter)
}

func (store *enrollmentStoreImpl) initializeLinked() {
	store.AddNullableFkIndex(store.symbolIdentity, store.stores.identity.symbolEnrollments)
	store.AddNullableFkIndex(store.symbolEdgeRouter, store.stores.edgeRouter.symbolEnrollments)
	store.AddNullableFkIndex(store.symbolTransitRouter, store.stores.transitRouter.symbolEnrollments)
}

func (store *enrollmentStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*Enrollment, error) {
	entity := &Enrollment{}
	if err := store.baseLoadOneById(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *enrollmentStoreImpl) LoadOneByToken(tx *bbolt.Tx, token string) (*Enrollment, error) {
	id := store.tokenIndex.Read(tx, []byte(token))
	if id != nil {
		return store.LoadOneById(tx, string(id))
	}
	return nil, nil
}

func (store *enrollmentStoreImpl) LoadOneByQuery(tx *bbolt.Tx, query string) (*Enrollment, error) {
	entity := &Enrollment{}
	if found, err := store.BaseLoadOneByQuery(tx, query, entity); !found || err != nil {
		return nil, err
	}
	return entity, nil
}
