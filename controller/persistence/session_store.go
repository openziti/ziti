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
	"github.com/openziti/foundation/storage/ast"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/openziti/foundation/util/errorz"
	"github.com/openziti/foundation/util/stringz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"time"
)

const (
	FieldSessionToken           = "token"
	FieldSessionApiSession      = "apiSession"
	FieldSessionService         = "service"
	FieldSessionIdentity        = "identity"
	FieldSessionType            = "type"
	FieldSessionServicePolicies = "servicePolicies"

	FieldSessionCertCert        = "cert"
	FieldSessionCertFingerprint = "fingerprint"
	FieldSessionCertValidFrom   = "validFrom"
	FieldSessionCertValidTo     = "validTo"

	SessionTypeDial = "Dial"
	SessionTypeBind = "Bind"
)

var validSessionTypes = []string{SessionTypeDial, SessionTypeBind}

type Session struct {
	boltz.BaseExtEntity
	Token           string
	IdentityId      string
	ApiSessionId    string
	ServiceId       string
	Type            string
	Certs           []*SessionCert
	ApiSession      *ApiSession
	ServicePolicies []string
}

func (entity *Session) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Token = bucket.GetStringOrError(FieldSessionToken)
	entity.ApiSessionId = bucket.GetStringOrError(FieldSessionApiSession)
	entity.ServiceId = bucket.GetStringOrError(FieldSessionService)
	entity.IdentityId = bucket.GetStringWithDefault(FieldSessionIdentity, "")
	entity.Type = bucket.GetStringWithDefault(FieldSessionType, "Dial")
	entity.ServicePolicies = bucket.GetStringList(FieldSessionServicePolicies)
}

func (entity *Session) SetValues(ctx *boltz.PersistContext) {
	if entity.Type == "" {
		entity.Type = SessionTypeDial
	}

	if !stringz.Contains(validSessionTypes, entity.Type) {
		ctx.Bucket.SetError(errorz.NewFieldError("invalid session type", FieldSessionType, entity.Type))
		return
	}

	entity.SetBaseValues(ctx)
	ctx.SetString(FieldSessionToken, entity.Token)
	ctx.SetString(FieldSessionApiSession, entity.ApiSessionId)
	ctx.SetString(FieldSessionService, entity.ServiceId)
	ctx.SetString(FieldSessionIdentity, entity.IdentityId)
	ctx.SetString(FieldSessionType, entity.Type)
	ctx.SetStringList(FieldSessionServicePolicies, entity.ServicePolicies)

	if ctx.FieldChecker == nil || ctx.FieldChecker.IsUpdated("sessionCerts") {
		mutateCtx := boltz.NewMutateContext(ctx.Bucket.Tx())
		for _, cert := range entity.Certs {
			ctx.Bucket.SetError(ctx.Store.CreateChild(mutateCtx, entity.Id, cert))
		}
	}

	if entity.ApiSession == nil {
		entity.ApiSession, _ = ctx.Store.(*sessionStoreImpl).stores.apiSession.LoadOneById(ctx.Bucket.Tx(), entity.ApiSessionId)
	}
}

func (entity *Session) GetEntityType() string {
	return EntityTypeSessions
}

type SessionCert struct {
	Id          string
	Cert        string
	Fingerprint string
	ValidFrom   time.Time
	ValidTo     time.Time
}

func (entity *SessionCert) GetId() string {
	return entity.Id
}

func (entity *SessionCert) SetId(id string) {
	entity.Id = id
}

func (entity *SessionCert) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.Cert = bucket.GetStringOrError(FieldSessionCertCert)
	entity.Fingerprint = bucket.GetStringOrError(FieldSessionCertFingerprint)
	entity.ValidFrom = bucket.GetTimeOrError(FieldSessionCertValidFrom)
	entity.ValidTo = bucket.GetTimeOrError(FieldSessionCertValidTo)
}

func (entity *SessionCert) SetValues(ctx *boltz.PersistContext) {
	ctx.Bucket.SetString(FieldSessionCertCert, entity.Cert, ctx.FieldChecker)
	ctx.Bucket.SetString(FieldSessionCertFingerprint, entity.Fingerprint, ctx.FieldChecker)
	ctx.Bucket.SetTime(FieldSessionCertValidFrom, entity.ValidFrom, ctx.FieldChecker)
	ctx.Bucket.SetTime(FieldSessionCertValidTo, entity.ValidTo, ctx.FieldChecker)
}

func (entity *SessionCert) GetEntityType() string {
	return EntityTypeSessionCerts
}

type SessionStore interface {
	Store
	LoadOneById(tx *bbolt.Tx, id string) (*Session, error)
	LoadOneByToken(tx *bbolt.Tx, token string) (*Session, error)
	LoadCerts(tx *bbolt.Tx, id string) ([]*SessionCert, error)
	GetTokenIndex() boltz.ReadIndex
}

func newSessionStore(stores *stores) *sessionStoreImpl {
	store := &sessionStoreImpl{
		baseStore: newBaseStore(stores, EntityTypeSessions),
	}
	store.InitImpl(store)
	return store
}

type sessionStoreImpl struct {
	*baseStore

	indexToken            boltz.ReadIndex
	symbolApiSession      boltz.EntitySymbol
	symbolService         boltz.EntitySymbol
	symbolServicePolicies boltz.EntitySetSymbol
}

func (store *sessionStoreImpl) NewStoreEntity() boltz.Entity {
	return &Session{}
}

func (store *sessionStoreImpl) GetTokenIndex() boltz.ReadIndex {
	return store.indexToken
}

func (store *sessionStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()

	symbolToken := store.AddSymbol(FieldSessionToken, ast.NodeTypeString)
	store.indexToken = store.AddUniqueIndex(symbolToken)

	store.symbolApiSession = store.AddFkSymbol(FieldSessionApiSession, store.stores.apiSession)
	store.symbolService = store.AddFkSymbol(FieldSessionService, store.stores.edgeService)
	store.symbolServicePolicies = store.AddFkSetSymbol(FieldSessionServicePolicies, store.stores.servicePolicy)
	store.AddSymbol(FieldSessionType, ast.NodeTypeString)

	store.AddFkConstraint(store.symbolApiSession, false, boltz.CascadeDelete)
	store.AddFkConstraint(store.symbolService, false, boltz.CascadeDelete)
}

func (store *sessionStoreImpl) initializeLinked() {}

func (store *sessionStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*Session, error) {
	entity := &Session{}
	if err := store.baseLoadOneById(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *sessionStoreImpl) LoadOneByToken(tx *bbolt.Tx, token string) (*Session, error) {
	id := store.indexToken.Read(tx, []byte(token))
	if id != nil {
		return store.LoadOneById(tx, string(id))
	}
	return nil, nil
}

func (store *sessionStoreImpl) LoadCerts(tx *bbolt.Tx, id string) ([]*SessionCert, error) {
	ids := store.ListChildIds(tx, id, EntityTypeSessionCerts)

	var result []*SessionCert
	for _, childId := range ids {
		sessionCert := &SessionCert{}
		found, err := store.BaseLoadOneChildById(tx, id, childId, sessionCert)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, errors.Errorf("session %v missing record for session cert %v", id, childId)
		}
		result = append(result, sessionCert)
	}
	return result, nil
}
