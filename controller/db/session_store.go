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
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

const (
	FieldSessionToken           = "token"
	FieldSessionApiSession      = "apiSession"
	FieldSessionService         = "service"
	FieldSessionIdentity        = "identity"
	FieldSessionType            = "type"
	FieldSessionServicePolicies = "servicePolicies"

	SessionTypeDial = "Dial"
	SessionTypeBind = "Bind"
)

var validSessionTypes = []string{SessionTypeDial, SessionTypeBind}

type Session struct {
	boltz.BaseExtEntity
	Token           string      `json:"-"`
	IdentityId      string      `json:"identityId"`
	ApiSessionId    string      `json:"apiSessionId"`
	ServiceId       string      `json:"serviceId"`
	Type            string      `json:"type"`
	ApiSession      *ApiSession `json:"-"`
	ServicePolicies []string    `json:"servicePolicies"`
}

func (entity *Session) GetEntityType() string {
	return EntityTypeSessions
}

var _ SessionStore = (*sessionStoreImpl)(nil)

type SessionStore interface {
	Store[*Session]
	LoadOneByToken(tx *bbolt.Tx, token string) (*Session, error)
	GetTokenIndex() boltz.ReadIndex
}

func newSessionStore(stores *stores) *sessionStoreImpl {
	store := &sessionStoreImpl{}
	store.baseStore = newBaseStore[*Session](stores, store)
	store.InitImpl(store)
	return store
}

type sessionStoreImpl struct {
	*baseStore[*Session]

	indexToken            boltz.ReadIndex
	symbolApiSession      boltz.EntitySymbol
	symbolService         boltz.EntitySymbol
	symbolServicePolicies boltz.EntitySetSymbol
}

func (store *sessionStoreImpl) NewEntity() *Session {
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
	sessionTypeSymbol := store.AddSymbol(FieldSessionType, ast.NodeTypeString)

	store.AddFkConstraint(store.symbolApiSession, false, boltz.CascadeCreateUpdate)
	store.AddFkConstraint(store.symbolService, false, boltz.CascadeDelete)
	store.AddConstraint(&sessionApiSessionIndex{
		apiSessionSymbol:  store.symbolApiSession,
		sessionTypeSymbol: sessionTypeSymbol,
		serviceSymbol:     store.symbolService,
		apiSessionStore:   store.stores.apiSession,
	})
}

func (store *sessionStoreImpl) initializeLinked() {}

func (*sessionStoreImpl) FillEntity(entity *Session, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Token = bucket.GetStringOrError(FieldSessionToken)
	entity.ApiSessionId = bucket.GetStringOrError(FieldSessionApiSession)
	entity.ServiceId = bucket.GetStringOrError(FieldSessionService)
	entity.IdentityId = bucket.GetStringWithDefault(FieldSessionIdentity, "")
	entity.Type = bucket.GetStringWithDefault(FieldSessionType, "Dial")
	entity.ServicePolicies = bucket.GetStringList(FieldSessionServicePolicies)
}

func (*sessionStoreImpl) PersistEntity(entity *Session, ctx *boltz.PersistContext) {
	if entity.Type == "" {
		entity.Type = SessionTypeDial
	}

	if !stringz.Contains(validSessionTypes, entity.Type) {
		ctx.Bucket.SetError(errorz.NewFieldError("invalid session type", FieldSessionType, entity.Type))
		return
	}

	entity.SetBaseValues(ctx)
	if ctx.IsCreate {
		ctx.SetString(FieldSessionToken, entity.Token)
		ctx.SetString(FieldSessionApiSession, entity.ApiSessionId)
		ctx.SetString(FieldSessionService, entity.ServiceId)

		sessionStore := ctx.Store.(*sessionStoreImpl)
		_, identityId := sessionStore.stores.apiSession.symbolIdentity.Eval(ctx.Tx(), []byte(entity.ApiSessionId))
		entity.IdentityId = string(identityId)

		ctx.SetString(FieldSessionIdentity, entity.IdentityId)
		ctx.SetString(FieldSessionType, entity.Type)
	}
	ctx.SetStringList(FieldSessionServicePolicies, entity.ServicePolicies)

	if entity.ApiSession == nil {
		entity.ApiSession, _ = ctx.Store.(*sessionStoreImpl).stores.apiSession.LoadOneById(ctx.Bucket.Tx(), entity.ApiSessionId)
	}
}

func (store *sessionStoreImpl) LoadOneByToken(tx *bbolt.Tx, token string) (*Session, error) {
	id := store.indexToken.Read(tx, []byte(token))
	if id != nil {
		return store.LoadOneById(tx, string(id))
	}
	return nil, boltz.NewNotFoundError(store.GetSingularEntityType(), "token", token)
}

type sessionApiSessionIndex struct {
	apiSessionSymbol  boltz.EntitySymbol
	serviceSymbol     boltz.EntitySymbol
	sessionTypeSymbol boltz.EntitySymbol
	apiSessionStore   boltz.Store
}

func (self *sessionApiSessionIndex) ProcessBeforeUpdate(*boltz.IndexingContext) {}

func (self *sessionApiSessionIndex) ProcessAfterUpdate(ctx *boltz.IndexingContext) {
	if ctx.IsCreate && !ctx.ErrHolder.HasError() {
		_, sessionType := self.sessionTypeSymbol.Eval(ctx.Tx(), ctx.RowId)
		_, apiSessionId := self.apiSessionSymbol.Eval(ctx.Tx(), ctx.RowId)
		_, serviceId := self.serviceSymbol.Eval(ctx.Tx(), ctx.RowId)

		if len(apiSessionId) == 0 {
			ctx.ErrHolder.SetError(errors.Errorf("index on %v.%v does not allow null or empty values",
				self.apiSessionSymbol.GetStore().GetEntityType(), self.apiSessionSymbol.GetName()))
			return
		}

		bucket := boltz.GetOrCreatePath(ctx.Tx(),
			RootBucket,
			boltz.IndexesBucket,
			EntityTypeApiSessions,
			EntityTypeSessions,
			string(apiSessionId),
			string(sessionType),
		)

		if existingSessionId := bucket.GetString(string(serviceId)); existingSessionId != nil {
			if self.sessionTypeSymbol.GetStore().IsEntityPresent(ctx.Tx(), *existingSessionId) {
				ctx.ErrHolder.SetError(errors.Errorf("session for api-session %v, service %v and type: %v already exists",
					string(apiSessionId), string(serviceId), string(sessionType)))
			}
		}

		bucket.SetString(string(serviceId), string(ctx.RowId), nil)
		ctx.ErrHolder.SetError(bucket.GetError())
	}
}

func (self *sessionApiSessionIndex) ProcessBeforeDelete(*boltz.IndexingContext) {}

func (self *sessionApiSessionIndex) Initialize(*bbolt.Tx, errorz.ErrorHolder) {}

func (self *sessionApiSessionIndex) CheckIntegrity(boltz.MutateContext, bool, func(err error, fixed bool)) error {
	return nil
}
