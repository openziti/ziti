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
	FieldApiSessionCertificateApiSession  = "apiSession"
	FieldApiSessionCertificateSubject     = "subject"
	FieldApiSessionCertificateFingerprint = "fingerprint"
	FieldApiSessionCertificateValidAfter  = "validAfter"
	FieldApiSessionCertificateValidBefore = "validBefore"
	FieldApiSessionCertificatePem         = "pem"
)

type ApiSessionCertificate struct {
	boltz.BaseExtEntity
	ApiSessionId string     `json:"apiSessionId"`
	Subject      string     `json:"subject"`
	Fingerprint  string     `json:"fingerprint"`
	ValidAfter   *time.Time `json:"validAfter"`
	ValidBefore  *time.Time `json:"validBefore"`
	PEM          string     `json:"pem"`
}

func (entity *ApiSessionCertificate) GetEntityType() string {
	return EntityTypeApiSessionCertificates
}

var _ ApiSessionCertificateStore = (*ApiSessionCertificateStoreImpl)(nil)

type ApiSessionCertificateStore interface {
	Store[*ApiSessionCertificate]
}

func newApiSessionCertificateStore(stores *stores) *ApiSessionCertificateStoreImpl {
	store := &ApiSessionCertificateStoreImpl{}
	store.baseStore = newBaseStore[*ApiSessionCertificate](stores, store)
	store.InitImpl(store)
	return store
}

type ApiSessionCertificateStoreImpl struct {
	*baseStore[*ApiSessionCertificate]
	symbolApiSession boltz.EntitySymbol
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

func (store *ApiSessionCertificateStoreImpl) NewEntity() *ApiSessionCertificate {
	return &ApiSessionCertificate{}
}

func (store *ApiSessionCertificateStoreImpl) FillEntity(entity *ApiSessionCertificate, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.ApiSessionId = bucket.GetStringOrError(FieldApiSessionCertificateApiSession)
	entity.Subject = bucket.GetStringOrError(FieldApiSessionCertificateSubject)
	entity.Fingerprint = bucket.GetStringOrError(FieldApiSessionCertificateFingerprint)
	entity.ValidAfter = bucket.GetTime(FieldApiSessionCertificateValidAfter)
	entity.ValidBefore = bucket.GetTime(FieldApiSessionCertificateValidBefore)
	entity.PEM = bucket.GetStringOrError(FieldApiSessionCertificatePem)
}

func (store *ApiSessionCertificateStoreImpl) PersistEntity(entity *ApiSessionCertificate, ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldApiSessionCertificateApiSession, entity.ApiSessionId)
	ctx.SetString(FieldApiSessionCertificateSubject, entity.Subject)
	ctx.SetString(FieldApiSessionCertificateFingerprint, entity.Fingerprint)
	ctx.SetTimeP(FieldApiSessionCertificateValidAfter, entity.ValidAfter)
	ctx.SetTimeP(FieldApiSessionCertificateValidBefore, entity.ValidBefore)
	ctx.SetString(FieldApiSessionCertificatePem, entity.PEM)
}
