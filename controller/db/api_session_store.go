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
	"github.com/kataras/go-events"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/controller/change"
	"go.etcd.io/bbolt"
	"strings"
	"time"
)

const (
	FieldApiSessionIdentity       = "identity"
	FieldApiSessionToken          = "token"
	FieldApiSessionConfigTypes    = "configTypes"
	FieldApiSessionIPAddress      = "ipAddress"
	FieldApiSessionMfaComplete    = "mfaComplete"
	FieldApiSessionMfaRequired    = "mfaRequired"
	FieldApiSessionLastActivityAt = "lastActivityAt"
	FieldApiSessionAuthenticator  = "authenticator"

	EventFullyAuthenticated events.EventName = "FULLY_AUTHENTICATED"

	EventualEventApiSessionDelete = "ApiSessionDelete"
)

type ApiSession struct {
	boltz.BaseExtEntity
	IdentityId      string    `json:"identityId"`
	Token           string    `json:"-"`
	IPAddress       string    `json:"ipAddress"`
	ConfigTypes     []string  `json:"configTypes"`
	MfaComplete     bool      `json:"mfaComplete"`
	MfaRequired     bool      `json:"mfaRequired"`
	LastActivityAt  time.Time `json:"lastActivityAt"`
	AuthenticatorId string    `json:"authenticatorId"`
}

func NewApiSession(identityId string) *ApiSession {
	return &ApiSession{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		IdentityId:    identityId,
		Token:         eid.New(),
	}
}

func (entity *ApiSession) GetEntityType() string {
	return EntityTypeApiSessions
}

var _ ApiSessionStore = (*apiSessionStoreImpl)(nil)

type ApiSessionStore interface {
	Store[*ApiSession]
	LoadOneByToken(tx *bbolt.Tx, token string) (*ApiSession, error)
	GetTokenIndex() boltz.ReadIndex
	GetCachedSessionId(tx *bbolt.Tx, apiSessionId, sessionType, serviceId string) *string
	GetEventsEmitter() events.EventEmmiter
}

func newApiSessionStore(stores *stores) *apiSessionStoreImpl {
	store := &apiSessionStoreImpl{
		eventsEmitter: events.New(),
	}
	store.baseStore = newBaseStore[*ApiSession](stores, store)
	stores.EventualEventer.AddEventualListener(EventualEventApiSessionDelete, store.onEventualDelete)
	store.InitImpl(store)
	return store
}

type apiSessionStoreImpl struct {
	*baseStore[*ApiSession]

	indexToken     boltz.ReadIndex
	symbolIdentity boltz.EntitySymbol
	eventsEmitter  events.EventEmmiter
}

func (store *apiSessionStoreImpl) NewEntity() *ApiSession {
	return &ApiSession{}
}

func (store *apiSessionStoreImpl) FillEntity(entity *ApiSession, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.IdentityId = bucket.GetStringOrError(FieldApiSessionIdentity)
	entity.Token = bucket.GetStringOrError(FieldApiSessionToken)
	entity.ConfigTypes = bucket.GetStringList(FieldApiSessionConfigTypes)
	entity.IPAddress = bucket.GetStringWithDefault(FieldApiSessionIPAddress, "")
	entity.MfaComplete = bucket.GetBoolWithDefault(FieldApiSessionMfaComplete, false)
	entity.MfaRequired = bucket.GetBoolWithDefault(FieldApiSessionMfaRequired, false)
	entity.AuthenticatorId = bucket.GetStringWithDefault(FieldApiSessionAuthenticator, "")
	lastActivityAt := bucket.GetTime(FieldApiSessionLastActivityAt) //not orError due to migration v18

	if lastActivityAt != nil {
		entity.LastActivityAt = *lastActivityAt
	}
}

func (store *apiSessionStoreImpl) PersistEntity(entity *ApiSession, ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldApiSessionIdentity, entity.IdentityId)
	ctx.SetString(FieldApiSessionToken, entity.Token)
	ctx.SetStringList(FieldApiSessionConfigTypes, entity.ConfigTypes)
	ctx.SetString(FieldApiSessionIPAddress, entity.IPAddress)
	ctx.SetBool(FieldApiSessionMfaComplete, entity.MfaComplete)
	ctx.SetBool(FieldApiSessionMfaRequired, entity.MfaRequired)
	ctx.SetString(FieldApiSessionAuthenticator, entity.AuthenticatorId)
	ctx.SetTimeP(FieldApiSessionLastActivityAt, &entity.LastActivityAt)
}

func (store *apiSessionStoreImpl) GetEventsEmitter() events.EventEmmiter {
	return store.eventsEmitter
}

func (store *apiSessionStoreImpl) onEventualDelete(db boltz.Db, name string, apiSessionId []byte) {
	idCollector := &sessionIdCollector{}
	indexPath := []string{RootBucket, boltz.IndexesBucket, EntityTypeApiSessions, EntityTypeSessions}
	err := db.View(func(tx *bbolt.Tx) error {
		path := append(indexPath, string(apiSessionId))
		if bucket := boltz.Path(tx, path...); bucket != nil {
			boltz.Traverse(bucket.Bucket, "/"+strings.Join(path, "/"), idCollector)
		}
		return nil
	})

	if err != nil {
		pfxlog.Logger().WithError(err).WithFields(map[string]interface{}{
			"eventName":    name,
			"apiSessionId": string(apiSessionId),
		}).Error("error querying for session associated to an api session during onEventualDelete")
	}

	for _, id := range idCollector.ids {
		changeContext := change.New().SetSourceType("events.emitter").SetChangeAuthorType(change.AuthorTypeController)
		err = db.Update(changeContext.NewMutateContext(), func(ctx boltz.MutateContext) error {
			if err := store.stores.session.DeleteById(ctx, id); err != nil {
				if boltz.IsErrNotFoundErr(err) {
					return nil
				}
				return err
			}
			return nil
		})

		if err != nil {
			pfxlog.Logger().WithError(err).WithFields(map[string]interface{}{
				"eventName":    name,
				"apiSessionId": string(apiSessionId),
				"sessionId":    id,
			}).Error("error deleting for session associated to an api session during onEventualDelete")
		}
	}

	changeContext := change.New().SetSourceType("events.emitter").SetChangeAuthorType(change.AuthorTypeController)
	err = db.Update(changeContext.NewMutateContext(), func(ctx boltz.MutateContext) error {
		if bucket := boltz.Path(ctx.Tx(), indexPath...); bucket != nil {
			if err := bucket.DeleteBucket(apiSessionId); err != nil {
				if err != bbolt.ErrBucketNotFound {
					return err
				}
			}
		}
		return nil
	})

	if err != nil {
		pfxlog.Logger().WithError(err).WithFields(map[string]interface{}{
			"eventName":    name,
			"apiSessionId": string(apiSessionId),
		}).Error("error deleting for api session index associated to an api session during onEventualDelete")
	}
}

func (store *apiSessionStoreImpl) Create(ctx boltz.MutateContext, entity *ApiSession) error {
	err := store.baseStore.Create(ctx, entity)

	if err == nil {
		if !entity.MfaRequired || entity.MfaComplete {
			ctx.AddCommitAction(func() {
				store.eventsEmitter.Emit(EventFullyAuthenticated, entity)
			})
		}

	}

	return err
}
func (store *apiSessionStoreImpl) Update(ctx boltz.MutateContext, entity *ApiSession, checker boltz.FieldChecker) error {
	err := store.baseStore.Update(ctx, entity, checker)

	if err == nil {
		if (checker == nil || checker.IsUpdated(FieldApiSessionMfaComplete)) && entity.MfaComplete {
			ctx.AddCommitAction(func() {
				store.eventsEmitter.Emit(EventFullyAuthenticated, entity)
			})
		}
	}

	return err
}

func (store *apiSessionStoreImpl) DeleteById(ctx boltz.MutateContext, id string) error {
	err := store.baseStore.DeleteById(ctx, id)

	if err == nil {
		if bboltEventualEventer, ok := store.baseStore.stores.EventualEventer.(*EventualEventerBbolt); ok {
			bboltEventualEventer.AddEventualEventWithCtx(ctx, EventualEventApiSessionDelete, []byte(id))
		} else {
			store.baseStore.stores.EventualEventer.AddEventualEvent(EventualEventApiSessionDelete, []byte(id))
		}
	}

	return err
}

func (store *apiSessionStoreImpl) GetTokenIndex() boltz.ReadIndex {
	return store.indexToken
}

func (store *apiSessionStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()
	symbolToken := store.AddSymbol(FieldApiSessionToken, ast.NodeTypeString)
	store.indexToken = store.AddUniqueIndex(symbolToken)
	store.symbolIdentity = store.AddFkSymbol(FieldApiSessionIdentity, store.stores.identity)
	store.AddSymbol(FieldApiSessionLastActivityAt, ast.NodeTypeDatetime)
	store.AddSymbol(FieldApiSessionAuthenticator, ast.NodeTypeString)
	store.AddSymbol(FieldApiSessionIdentity, ast.NodeTypeString)
	store.AddSymbol(FieldApiSessionIPAddress, ast.NodeTypeString)
	store.AddSymbol(FieldApiSessionLastActivityAt, ast.NodeTypeDatetime)

	store.AddFkConstraint(store.symbolIdentity, false, boltz.CascadeDelete)
}

func (store *apiSessionStoreImpl) initializeLinked() {
}

func (store *apiSessionStoreImpl) LoadOneByToken(tx *bbolt.Tx, token string) (*ApiSession, error) {
	id := store.indexToken.Read(tx, []byte(token))
	if id != nil {
		return store.LoadOneById(tx, string(id))
	}
	return nil, boltz.NewNotFoundError(store.GetSingularEntityType(), "token", token)
}

func (store *apiSessionStoreImpl) GetCachedSessionId(tx *bbolt.Tx, apiSessionId, sessionType, serviceId string) *string {
	bucket := boltz.Path(tx,
		RootBucket, boltz.IndexesBucket,
		EntityTypeApiSessions, EntityTypeSessions,
		apiSessionId, sessionType,
	)

	if bucket != nil {
		return bucket.GetString(serviceId)
	}

	return nil
}

type UpdateLastActivityAtChecker struct{}

func (u UpdateLastActivityAtChecker) IsUpdated(field string) bool {
	return field == FieldApiSessionLastActivityAt
}

type sessionIdCollector struct {
	ids []string
}

func (self *sessionIdCollector) VisitBucket(string, []byte, *bbolt.Bucket) bool {
	return true
}

func (self *sessionIdCollector) VisitKeyValue(_ string, _, value []byte) bool {
	if sessionId := boltz.FieldToString(boltz.GetTypeAndValue(value)); sessionId != nil {
		self.ids = append(self.ids, *sessionId)
	}
	return true
}
