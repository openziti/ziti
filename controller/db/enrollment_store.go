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
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
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
	Token           string     `json:"token"`
	Method          string     `json:"method"`
	IdentityId      *string    `json:"identityId"`
	TransitRouterId *string    `json:"transitRouterId"`
	EdgeRouterId    *string    `json:"edgeRouterId"`
	ExpiresAt       *time.Time `json:"expiresAt"`
	IssuedAt        *time.Time `json:"issuedAt"`
	CaId            *string    `json:"caId"`
	Username        *string    `json:"username"`
	Jwt             string     `json:"-"`
}

func (entity *Enrollment) GetEntityType() string {
	return EntityTypeEnrollments
}

var enrollmentFieldMappings = map[string]string{
	FieldEnrollIdentity:      "identityId",
	FieldEnrollEdgeRouter:    "edgeRouterId",
	FieldEnrollTransitRouter: "transitRouterId",
}

var _ EnrollmentStore = (*enrollmentStoreImpl)(nil)

type EnrollmentStore interface {
	Store[*Enrollment]
	LoadOneByToken(tx *bbolt.Tx, token string) (*Enrollment, error)
}

func newEnrollmentStore(stores *stores) *enrollmentStoreImpl {
	store := &enrollmentStoreImpl{}
	store.baseStore = newBaseStore[*Enrollment](stores, store)
	store.InitImpl(store)

	return store
}

type enrollmentStoreImpl struct {
	*baseStore[*Enrollment]
	tokenIndex          boltz.ReadIndex
	symbolIdentity      boltz.EntitySymbol
	symbolEdgeRouter    boltz.EntitySymbol
	symbolTransitRouter boltz.EntitySymbol
	symbolCa            boltz.EntitySymbol
}

func (store *enrollmentStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()

	symbolToken := store.AddSymbol(FieldEnrollmentToken, ast.NodeTypeString)
	store.tokenIndex = store.AddUniqueIndex(symbolToken)
	store.symbolIdentity = store.AddFkSymbol(FieldEnrollIdentity, store.stores.identity)
	store.symbolEdgeRouter = store.AddFkSymbol(FieldEnrollEdgeRouter, store.stores.edgeRouter)
	store.symbolTransitRouter = store.AddFkSymbol(FieldEnrollTransitRouter, store.stores.transitRouter)
	store.symbolCa = store.AddFkSymbol(FieldEnrollmentCaId, store.stores.ca)
}

func (store *enrollmentStoreImpl) initializeLinked() {
	store.AddNullableFkIndex(store.symbolIdentity, store.stores.identity.symbolEnrollments)
	store.AddNullableFkIndex(store.symbolEdgeRouter, store.stores.edgeRouter.symbolEnrollments)
	store.AddNullableFkIndex(store.symbolTransitRouter, store.stores.transitRouter.symbolEnrollments)
	store.AddNullableFkIndex(store.symbolCa, store.stores.ca.symbolEnrollments)
}

func (store *enrollmentStoreImpl) NewEntity() *Enrollment {
	return &Enrollment{}
}

func (store *enrollmentStoreImpl) FillEntity(entity *Enrollment, bucket *boltz.TypedBucket) {
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

func (store *enrollmentStoreImpl) PersistEntity(entity *Enrollment, ctx *boltz.PersistContext) {
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

func (store *enrollmentStoreImpl) LoadOneByToken(tx *bbolt.Tx, token string) (*Enrollment, error) {
	id := store.tokenIndex.Read(tx, []byte(token))
	if id != nil {
		return store.LoadOneById(tx, string(id))
	}
	return nil, nil
}
