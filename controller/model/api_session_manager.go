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

package model

import (
	"fmt"
	"time"

	"github.com/lucsky/cuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/models"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

func NewApiSessionManager(env Env) *ApiSessionManager {
	manager := &ApiSessionManager{
		baseEntityManager: newBaseEntityManager[*ApiSession, *db.ApiSession](env, env.GetStores().ApiSession),
	}

	manager.HeartbeatCollector = NewHeartbeatCollector(env, env.GetConfig().Api.ActivityUpdateBatchSize, env.GetConfig().Api.ActivityUpdateInterval, manager.heartbeatFlush)

	manager.impl = manager

	return manager
}

type ApiSessionManager struct {
	baseEntityManager[*ApiSession, *db.ApiSession]
	HeartbeatCollector *HeartbeatCollector
}

func (self *ApiSessionManager) newModelEntity() *ApiSession {
	return &ApiSession{}
}

func (self *ApiSessionManager) Create(ctx boltz.MutateContext, entity *ApiSession, sessionCerts []*ApiSessionCertificate) (string, error) {
	var apiSessionId string
	err := self.env.GetDbProvider().GetDb().Update(ctx, func(ctx boltz.MutateContext) error {
		var err error
		apiSessionId, err = self.CreateInCtx(ctx, entity, sessionCerts)
		return err
	})
	if err != nil {
		return "", err
	}
	return apiSessionId, nil
}

func (self *ApiSessionManager) CreateInCtx(ctx boltz.MutateContext, entity *ApiSession, sessionCerts []*ApiSessionCertificate) (string, error) {
	entity.Id = cuid.New() //use cuids which are longer than shortids but are monotonic
	apiSessionId, err := self.createEntityInTx(ctx, entity)

	if err != nil {
		return "", err
	}

	for _, sessionCert := range sessionCerts {
		sessionCert.ApiSessionId = apiSessionId
		if _, err = self.env.GetManagers().ApiSessionCertificate.createEntityInTx(ctx, sessionCert); err != nil {
			return "", err
		}
	}

	self.MarkLastActivityById(apiSessionId)

	return apiSessionId, err
}

func (self *ApiSessionManager) Read(id string) (*ApiSession, error) {
	modelApiSession := &ApiSession{}
	if err := self.readEntity(id, modelApiSession); err != nil {
		return nil, err
	}
	return modelApiSession, nil
}

func (self *ApiSessionManager) ReadByToken(token string) (*ApiSession, error) {
	modelApiSession := &ApiSession{}
	tokenIndex := self.env.GetStores().ApiSession.GetTokenIndex()
	if err := self.readEntityWithIndex("token", []byte(token), tokenIndex, modelApiSession); err != nil {
		return nil, err
	}
	return modelApiSession, nil
}

func (self *ApiSessionManager) ReadInTx(tx *bbolt.Tx, id string) (*ApiSession, error) {
	modelApiSession := &ApiSession{}
	if err := self.readEntityInTx(tx, id, modelApiSession); err != nil {
		return nil, err
	}
	return modelApiSession, nil
}

func (self *ApiSessionManager) IsUpdated(_ string) bool {
	return false
}

func (self *ApiSessionManager) Update(apiSession *ApiSession, ctx *change.Context) error {
	return self.updateEntity(apiSession, self, ctx.NewMutateContext())
}

func (self *ApiSessionManager) UpdateWithFieldChecker(apiSession *ApiSession, fieldChecker boltz.FieldChecker, ctx *change.Context) error {
	return self.updateEntity(apiSession, fieldChecker, ctx.NewMutateContext())
}

func (self *ApiSessionManager) MfaCompleted(apiSession *ApiSession, ctx *change.Context) error {
	apiSession.MfaComplete = true
	return self.updateEntity(apiSession, &OrFieldChecker{NewFieldChecker(db.FieldApiSessionMfaComplete), self}, ctx.NewMutateContext())
}

func (self *ApiSessionManager) Delete(id string, ctx *change.Context) error {
	return self.deleteEntity(id, ctx)
}

func (self *ApiSessionManager) DeleteBatch(id []string, ctx *change.Context) error {
	return self.deleteEntityBatch(id, ctx)
}

// MarkLastActivityById marks the "last activity" of an API Session. This will store a cached "LastUpdatedAt" value for
// an API Session. This data will be used to populate information for API Sessions and will be persisted to the data
// store at a future time in bulk.
func (self *ApiSessionManager) MarkLastActivityById(apiSessionId string) {
	self.HeartbeatCollector.Mark(apiSessionId)
}

// MarkLastActivityByTokens returns the ids of identities that were affected, tokens that were not found if any or an error.
// Marking "last activity" will store a cached "LastUpdatedAt" value for an API Session. This data will be used to
// populate information for API Sessions and will be persisted to the data store at a future time in bulk.
func (self *ApiSessionManager) MarkLastActivityByTokens(tokens ...string) ([]string, []string, error) {
	var notFoundTokens []string
	store := self.env.GetStores().ApiSession

	var apiSessions []*db.ApiSession
	identityIds := map[string]struct{}{}

	err := self.GetDb().View(func(tx *bbolt.Tx) error {
		for _, token := range tokens {
			apiSession, err := store.LoadOneByToken(tx, token)
			if err != nil {
				if boltz.IsErrNotFoundErr(err) {
					notFoundTokens = append(notFoundTokens, token)
					continue
				} else {
					return err
				}
			}
			apiSessions = append(apiSessions, apiSession)
		}
		return nil
	})

	for _, apiSession := range apiSessions {
		self.MarkLastActivityById(apiSession.Id)
		identityIds[apiSession.IdentityId] = struct{}{}
	}

	var uniqueIdentityIds []string
	for identityId := range identityIds {
		uniqueIdentityIds = append(uniqueIdentityIds, identityId)
	}

	return uniqueIdentityIds, notFoundTokens, err
}

func (self *ApiSessionManager) heartbeatFlush(beats []*Heartbeat) {
	changeCtx := change.New().SetSourceType("heartbeat.flush").SetChangeAuthorType(change.AuthorTypeController)
	err := self.GetDb().Batch(changeCtx.NewMutateContext(), func(ctx boltz.MutateContext) error {
		store := self.env.GetStores().ApiSession

		for _, beat := range beats {
			err := store.Update(ctx, &db.ApiSession{
				BaseExtEntity: boltz.BaseExtEntity{
					Id: beat.ApiSessionId,
				},
				LastActivityAt: beat.LastActivityAt,
			}, db.UpdateLastActivityAtChecker{})

			if err != nil {
				pfxlog.Logger().Errorf("could not flush heartbeat activity for api session id %s: %v", beat.ApiSessionId, err)
			}
		}

		return nil
	})

	if err != nil {
		pfxlog.Logger().Errorf("could not flush heartbeat activity: %v", err)
	}
}

func (self *ApiSessionManager) Stream(query string, collect func(*ApiSession, error) error) error {
	filter, err := ast.Parse(self.Store, query)

	if err != nil {
		return fmt.Errorf("could not parse query for streaming api sessions: %v", err)
	}

	return self.env.GetDbProvider().GetDb().View(func(tx *bbolt.Tx) error {
		for cursor := self.Store.IterateIds(tx, filter); cursor.IsValid(); cursor.Next() {
			current := cursor.Current()

			apiSession, err := self.ReadInTx(tx, string(current))
			if err := collect(apiSession, err); err != nil {
				return err
			}
		}
		return collect(nil, nil)
	})
}

func (self *ApiSessionManager) StreamIds(query string, collect func(string, error) error) error {
	filter, err := ast.Parse(self.Store, query)

	if err != nil {
		return fmt.Errorf("could not parse query for streaming api sessions ids: %v", err)
	}

	return self.env.GetDbProvider().GetDb().View(func(tx *bbolt.Tx) error {
		for cursor := self.Store.IterateIds(tx, filter); cursor.IsValid(); cursor.Next() {
			current := cursor.Current()
			if err := collect(string(current), err); err != nil {
				return err
			}
		}
		return nil
	})
}

func (self *ApiSessionManager) Query(query string) (*ApiSessionListResult, error) {
	result := &ApiSessionListResult{manager: self}
	err := self.ListWithHandler(query, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (self *ApiSessionManager) VisitFingerprintsForApiSessionId(apiSessionId string, visitor func(fingerprint string) bool) error {
	return self.GetDb().View(func(tx *bbolt.Tx) error {
		apiSession, err := self.ReadInTx(tx, apiSessionId)
		if err != nil {
			return errors.Wrapf(err, "could not query fingerprints by api session id [%s]", apiSessionId)
		}

		return self.VisitFingerprintsForApiSession(tx, apiSession.IdentityId, apiSessionId, visitor)
	})
}

func (self *ApiSessionManager) VisitFingerprintsForApiSession(tx *bbolt.Tx, identityId, apiSessionId string, visitor func(fingerprint string) bool) error {
	if stopVisiting, err := self.env.GetManagers().Identity.VisitIdentityAuthenticatorFingerprints(tx, identityId, visitor); stopVisiting || err != nil {
		return err
	}

	apiSessionCerts, err := self.env.GetManagers().ApiSessionCertificate.ReadByApiSessionId(tx, apiSessionId)
	if err != nil {
		return err
	}

	now := time.Now()
	for _, apiSessionCert := range apiSessionCerts {
		if apiSessionCert.ValidAfter != nil && now.After(*apiSessionCert.ValidAfter) &&
			apiSessionCert.ValidBefore != nil && now.Before(*apiSessionCert.ValidBefore) {
			if visitor(apiSessionCert.Fingerprint) {
				return nil
			}
		}
	}

	return nil
}

func (self *ApiSessionManager) DeleteByIdentityId(identityId string, changeCtx *change.Context) error {
	return self.GetEnv().GetDbProvider().GetDb().Update(changeCtx.NewMutateContext(), func(ctx boltz.MutateContext) error {
		query := fmt.Sprintf(`%s = "%s"`, db.FieldApiSessionIdentity, identityId)
		return self.Store.DeleteWhere(ctx, query)
	})
}

type ApiSessionListResult struct {
	manager     *ApiSessionManager
	ApiSessions []*ApiSession
	models.QueryMetaData
}

func (result *ApiSessionListResult) collect(tx *bbolt.Tx, ids []string, queryMetaData *models.QueryMetaData) error {
	result.QueryMetaData = *queryMetaData
	for _, key := range ids {
		ApiSession, err := result.manager.ReadInTx(tx, key)
		if err != nil {
			return err
		}
		result.ApiSessions = append(result.ApiSessions, ApiSession)
	}
	return nil
}
