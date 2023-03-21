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

package persistence

import (
	"github.com/kataras/go-events"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/eid"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
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
	IdentityId      string
	Token           string
	IPAddress       string
	ConfigTypes     []string
	MfaComplete     bool
	MfaRequired     bool
	LastActivityAt  time.Time
	AuthenticatorId string
}

func NewApiSession(identityId string) *ApiSession {
	return &ApiSession{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		IdentityId:    identityId,
		Token:         eid.New(),
	}
}

func (entity *ApiSession) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
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

func (entity *ApiSession) SetValues(ctx *boltz.PersistContext) {
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

func (entity *ApiSession) GetEntityType() string {
	return EntityTypeApiSessions
}

type ApiSessionStore interface {
	Store
	LoadOneById(tx *bbolt.Tx, id string) (*ApiSession, error)
	LoadOneByToken(tx *bbolt.Tx, token string) (*ApiSession, error)
	LoadOneByQuery(tx *bbolt.Tx, query string) (*ApiSession, error)
	GetTokenIndex() boltz.ReadIndex
	GetCachedSessionId(tx *bbolt.Tx, apiSessionId, sessionType, serviceId string) *string
}

func newApiSessionStore(stores *stores) *apiSessionStoreImpl {
	store := &apiSessionStoreImpl{
		baseStore: newBaseStore(stores, EntityTypeApiSessions),
	}

	stores.EventualEventer.AddEventualListener(EventualEventApiSessionDelete, store.onEventualDelete)
	store.InitImpl(store)
	return store
}

type apiSessionStoreImpl struct {
	*baseStore

	indexToken     boltz.ReadIndex
	symbolIdentity boltz.EntitySymbol
}

func (store *apiSessionStoreImpl) onEventualDelete(name string, apiSessionId []byte) {
	idCollector := &sessionIdCollector{}
	indexPath := []string{db.RootBucket, boltz.IndexesBucket, EntityTypeApiSessions, EntityTypeSessions}
	err := store.stores.DbProvider.GetDb().View(func(tx *bbolt.Tx) error {
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
		err = store.stores.DbProvider.GetDb().Update(func(tx *bbolt.Tx) error {
			ctx := boltz.NewMutateContext(tx)
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

	err = store.stores.DbProvider.GetDb().Update(func(tx *bbolt.Tx) error {
		if bucket := boltz.Path(tx, indexPath...); bucket != nil {
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

func (store *apiSessionStoreImpl) Create(ctx boltz.MutateContext, entity boltz.Entity) error {
	err := store.baseStore.Create(ctx, entity)

	if err == nil {
		if apiSession, ok := entity.(*ApiSession); ok && apiSession != nil {
			if !apiSession.MfaRequired || apiSession.MfaComplete {
				ctx.AddEvent(store, EventFullyAuthenticated, apiSession)
			}
		}
	}

	return err
}
func (store *apiSessionStoreImpl) Update(ctx boltz.MutateContext, entity boltz.Entity, checker boltz.FieldChecker) error {
	err := store.baseStore.Update(ctx, entity, checker)

	if err == nil {
		if apiSession, ok := entity.(*ApiSession); ok && apiSession != nil {
			if (checker == nil || checker.IsUpdated(FieldApiSessionMfaComplete)) && apiSession.MfaComplete {
				ctx.AddEvent(store, EventFullyAuthenticated, apiSession)
			}
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

func (store *apiSessionStoreImpl) NewStoreEntity() boltz.Entity {
	return &ApiSession{}
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
	return nil, boltz.NewNotFoundError(store.GetSingularEntityType(), "token", token)
}

func (store *apiSessionStoreImpl) LoadOneByQuery(tx *bbolt.Tx, query string) (*ApiSession, error) {
	entity := &ApiSession{}
	if found, err := store.BaseLoadOneByQuery(tx, query, entity); !found || err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *apiSessionStoreImpl) GetCachedSessionId(tx *bbolt.Tx, apiSessionId, sessionType, serviceId string) *string {
	bucket := boltz.Path(tx,
		db.RootBucket, boltz.IndexesBucket,
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
