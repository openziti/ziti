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
	"github.com/openziti/edge/eid"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	"go.etcd.io/bbolt"
	"time"
)

const (
	FieldApiSessionCertificateApiSession  = "apiSession"
	FieldApiSessionCertificateSubject     = "subject"
	FieldApiSessionCertificateFingerprint = "fingerprint"
	FieldApiSessionCertificateValidAfter  = "validAfter"
	FieldApiSessionCertificateValidBefore = "validBefore"
	FieldApiSessionCertificatePem         = "pem"
)

type ApiSessionCertificate struct {
	boltz.BaseExtEntity
	ApiSessionId string
	Subject      string
	Fingerprint  string
	ValidAfter   *time.Time
	ValidBefore  *time.Time
	PEM          string
}

func NewApiSessionCertificate(apiSessionId string) *ApiSessionCertificate {
	return &ApiSessionCertificate{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		ApiSessionId:  apiSessionId,
		Subject:       eid.New(),
	}
}

func (entity *ApiSessionCertificate) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.ApiSessionId = bucket.GetStringOrError(FieldApiSessionCertificateApiSession)
	entity.Subject = bucket.GetStringOrError(FieldApiSessionCertificateSubject)
	entity.Fingerprint = bucket.GetStringOrError(FieldApiSessionCertificateFingerprint)
	entity.ValidAfter = bucket.GetTime(FieldApiSessionCertificateValidAfter)
	entity.ValidBefore = bucket.GetTime(FieldApiSessionCertificateValidBefore)
	entity.PEM = bucket.GetStringOrError(FieldApiSessionCertificatePem)
}

func (entity *ApiSessionCertificate) SetValues(ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldApiSessionCertificateApiSession, entity.ApiSessionId)
	ctx.SetString(FieldApiSessionCertificateSubject, entity.Subject)
	ctx.SetString(FieldApiSessionCertificateFingerprint, entity.Fingerprint)
	ctx.SetTimeP(FieldApiSessionCertificateValidAfter, entity.ValidAfter)
	ctx.SetTimeP(FieldApiSessionCertificateValidBefore, entity.ValidBefore)
	ctx.SetString(FieldApiSessionCertificatePem, entity.PEM)
}

func (entity *ApiSessionCertificate) GetEntityType() string {
	return EntityTypeApiSessionCertificates
}

type ApiSessionCertificateStore interface {
	Store
	LoadOneById(tx *bbolt.Tx, id string) (*ApiSessionCertificate, error)
	LoadOneByQuery(tx *bbolt.Tx, query string) (*ApiSessionCertificate, error)
}

func newApiSessionCertificateStore(stores *stores) *ApiSessionCertificateStoreImpl {
	store := &ApiSessionCertificateStoreImpl{
		baseStore: newBaseStore(stores, EntityTypeApiSessionCertificates),
	}
	store.InitImpl(store)
	return store
}

type ApiSessionCertificateStoreImpl struct {
	*baseStore
	symbolApiSession boltz.EntitySymbol
}

func (store *ApiSessionCertificateStoreImpl) NewStoreEntity() boltz.Entity {
	return &ApiSessionCertificate{}
}

func (store *ApiSessionCertificateStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()
	store.AddSymbol(FieldApiSessionCertificateApiSession, ast.NodeTypeString)
	store.AddSymbol(FieldApiSessionCertificateSubject, ast.NodeTypeString)
	store.AddSymbol(FieldApiSessionCertificateFingerprint, ast.NodeTypeString)
	store.symbolApiSession = store.AddFkSymbol(FieldApiSessionCertificateApiSession, store.stores.apiSession)

	store.AddFkConstraint(store.symbolApiSession, false, boltz.CascadeDelete)
}

func (store *ApiSessionCertificateStoreImpl) initializeLinked() {
}

func (store *ApiSessionCertificateStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*ApiSessionCertificate, error) {
	entity := &ApiSessionCertificate{}
	if err := store.baseLoadOneById(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *ApiSessionCertificateStoreImpl) LoadOneByQuery(tx *bbolt.Tx, query string) (*ApiSessionCertificate, error) {
	entity := &ApiSessionCertificate{}
	if found, err := store.BaseLoadOneByQuery(tx, query, entity); !found || err != nil {
		return nil, err
	}
	return entity, nil
}
