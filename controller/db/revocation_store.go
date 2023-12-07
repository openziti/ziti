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
	"time"
)

const (
	FieldRevocationExpiresAt = "expiresAt"
)

type Revocation struct {
	boltz.BaseExtEntity
	ExpiresAt time.Time `json:"expiresAt"`
}

func (r Revocation) GetEntityType() string {
	return EntityTypeRevocations
}

var _ RevocationStore = (*revocationStoreImpl)(nil)

type RevocationStore interface {
	Store[*Revocation]
}

func newRevocationStore(stores *stores) *revocationStoreImpl {
	store := &revocationStoreImpl{}
	store.baseStore = newBaseStore[*Revocation](stores, store)
	store.InitImpl(store)
	return store
}

type revocationStoreImpl struct {
	*baseStore[*Revocation]
}

func (store *revocationStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()
	store.AddSymbol(FieldRevocationExpiresAt, ast.NodeTypeDatetime)
}

func (store *revocationStoreImpl) initializeLinked() {}

func (store *revocationStoreImpl) NewEntity() *Revocation {
	return &Revocation{}
}

func (store *revocationStoreImpl) FillEntity(entity *Revocation, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.ExpiresAt = bucket.GetTimeOrError(FieldRevocationExpiresAt)
}

func (store *revocationStoreImpl) PersistEntity(entity *Revocation, ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetTimeP(FieldRevocationExpiresAt, &entity.ExpiresAt)
}
