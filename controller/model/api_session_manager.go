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
	"github.com/lucsky/cuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"time"
)

func NewApiSessionManager(env Env) *ApiSessionManager {
	manager := &ApiSessionManager{
		baseEntityManager: newBaseEntityManager(env, env.GetStores().ApiSession),
	}

	manager.HeartbeatCollector = NewHeartbeatCollector(env, env.GetConfig().Api.ActivityUpdateBatchSize, env.GetConfig().Api.ActivityUpdateInterval, manager.heartbeatFlush)

	manager.impl = manager

	return manager
}

type ApiSessionManager struct {
	baseEntityManager
	HeartbeatCollector *HeartbeatCollector
}

func (self *ApiSessionManager) newModelEntity() edgeEntity {
	return &ApiSession{}
}

func (self *ApiSessionManager) Create(entity *ApiSession, sessionCerts []*ApiSessionCertificate) (string, error) {
	entity.Id = cuid.New() //use cuids which are longer than shortids but are monotonic

	var apiSessionId string
	err := self.env.GetDbProvider().GetDb().Update(func(tx *bbolt.Tx) error {
		var err error
		ctx := boltz.NewMutateContext(tx)
		apiSessionId, err = self.createEntityInTx(ctx, entity)

		if err != nil {
			return err
		}

		for _, sessionCert := range sessionCerts {
			sessionCert.ApiSessionId = apiSessionId
			_, err := self.env.GetManagers().ApiSessionCertificate.createEntityInTx(ctx, sessionCert)

			if err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		self.MarkActivityById(apiSessionId)
	}

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

func (self *ApiSessionManager) readInTx(tx *bbolt.Tx, id string) (*ApiSession, error) {
	modelApiSession := &ApiSession{}
	if err := self.readEntityInTx(tx, id, modelApiSession); err != nil {
		return nil, err
	}
	return modelApiSession, nil
}

func (self *ApiSessionManager) IsUpdated(_ string) bool {
	return false
}

func (self *ApiSessionManager) Update(apiSession *ApiSession) error {
	return self.updateEntity(apiSession, self)
}

func (self *ApiSessionManager) UpdateWithFieldChecker(apiSession *ApiSession, fieldChecker boltz.FieldChecker) error {
	return self.updateEntity(apiSession, fieldChecker)
}

func (self *ApiSessionManager) MfaCompleted(apiSession *ApiSession) error {
	apiSession.MfaComplete = true
	return self.updateEntity(apiSession, &OrFieldChecker{NewFieldChecker(persistence.FieldApiSessionMfaComplete), self})
}

func (self *ApiSessionManager) Delete(id string) error {
	return self.deleteEntity(id)
}

func (self *ApiSessionManager) DeleteBatch(id []string) error {
	return self.deleteEntityBatch(id)
}

func (self *ApiSessionManager) MarkActivityById(apiSessionId string) {
	self.HeartbeatCollector.Mark(apiSessionId)
}

// MarkActivityByTokens returns tokens that were not found if any and/or an error.
func (self *ApiSessionManager) MarkActivityByTokens(tokens ...string) ([]string, error) {
	var notFoundTokens []string
	store := self.Store.(persistence.ApiSessionStore)

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
			self.HeartbeatCollector.Mark(apiSession.Id)
			self.env.GetManagers().Identity.SetActive(apiSession.IdentityId)
		}
		return nil
	})

	return notFoundTokens, err
}

func (self *ApiSessionManager) heartbeatFlush(beats []*Heartbeat) {
	err := self.GetDb().Batch(func(tx *bbolt.Tx) error {
		store := self.Store.(persistence.ApiSessionStore)
		mutCtx := boltz.NewMutateContext(tx)

		for _, beat := range beats {
			err := store.Update(mutCtx, &persistence.ApiSession{
				BaseExtEntity: boltz.BaseExtEntity{
					Id: beat.ApiSessionId,
				},
				LastActivityAt: beat.LastActivityAt,
			}, persistence.UpdateLastActivityAtChecker{})

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
		return fmt.Errorf("could not parse query for streaming api sesions: %v", err)
	}

	return self.env.GetDbProvider().GetDb().View(func(tx *bbolt.Tx) error {
		for cursor := self.Store.IterateIds(tx, filter); cursor.IsValid(); cursor.Next() {
			current := cursor.Current()

			apiSession, err := self.readInTx(tx, string(current))
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
		return fmt.Errorf("could not parse query for streaming api sesions ids: %v", err)
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
		apiSession, err := self.readInTx(tx, apiSessionId)
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

func (self *ApiSessionManager) DeleteByIdentityId(identityId string) error {
	return self.GetEnv().GetDbProvider().GetDb().Update(func(tx *bbolt.Tx) error {
		query := fmt.Sprintf(`%s = "%s"`, persistence.FieldApiSessionIdentity, identityId)
		return self.Store.DeleteWhere(boltz.NewMutateContext(tx), query)
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
		ApiSession, err := result.manager.readInTx(tx, key)
		if err != nil {
			return err
		}
		result.ApiSessions = append(result.ApiSessions, ApiSession)
	}
	return nil
}
