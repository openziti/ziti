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
	"fmt"
	"github.com/kataras/go-events"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/eid"
	"github.com/openziti/foundation/storage/ast"
	"github.com/openziti/foundation/storage/boltz"
	"go.etcd.io/bbolt"
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

	EventFullyAuthenticated       events.EventName = "FULLY_AUTHENTICATED"
	EventualEventApiSessionDelete                  = "ApiSessionDelete"
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

func (store *apiSessionStoreImpl) onEventualDelete(name string, data []byte) {
	var ids []string
	err := store.stores.DbProvider.GetDb().View(func(tx *bbolt.Tx) error {
		query := fmt.Sprintf(`%s = "%s"`, FieldSessionApiSession, string(data))
		var err error
		ids, _, err = store.stores.session.QueryIds(tx, query)
		return err
	})

	if err != nil {
		pfxlog.Logger().WithError(err).WithFields(map[string]interface{}{
			"eventName":    name,
			"apiSessionId": string(data),
		}).Error("error querying for session associated to an api session during onEventualDelete")
	}

	for _, id := range ids {
		_ = store.stores.DbProvider.GetDb().Update(func(tx *bbolt.Tx) error {
			ctx := boltz.NewMutateContext(tx)
			err := store.stores.session.DeleteById(ctx, id)

			if err != nil {
				pfxlog.Logger().WithError(err).WithFields(map[string]interface{}{
					"eventName":    name,
					"apiSessionId": string(data),
					"sessionId":    id,
				}).Error("error deleting for session associated to an api session during onEventualDelete")
			}

			return nil
		})
	}
}

func (store *apiSessionStoreImpl) Create(ctx boltz.MutateContext, entity boltz.Entity) error {
	err := store.baseStore.Create(ctx, entity)

	if err == nil {
		if apiSession, ok := entity.(*ApiSession); ok && apiSession != nil {
			if apiSession.MfaRequired == false || apiSession.MfaComplete == true {
				store.Emit(EventFullyAuthenticated, apiSession)
			}
		}
	}

	return err
}
func (store *apiSessionStoreImpl) Update(ctx boltz.MutateContext, entity boltz.Entity, checker boltz.FieldChecker) error {
	err := store.baseStore.Update(ctx, entity, checker)

	if err == nil {
		if apiSession, ok := entity.(*ApiSession); ok && apiSession != nil {
			if (checker == nil || checker.IsUpdated(FieldApiSessionMfaComplete)) && apiSession.MfaComplete == true {
				store.Emit(EventFullyAuthenticated, apiSession)
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

type UpdateLastActivityAtChecker struct{}

func (u UpdateLastActivityAtChecker) IsUpdated(field string) bool {
	return field == FieldApiSessionLastActivityAt
}
