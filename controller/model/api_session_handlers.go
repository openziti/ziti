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

package model

import (
	"fmt"
	"github.com/lucsky/cuid"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/storage/ast"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"time"
)

func NewApiSessionHandler(env Env) *ApiSessionHandler {
	handler := &ApiSessionHandler{
		baseHandler: newBaseHandler(env, env.GetStores().ApiSession),
	}
	handler.impl = handler

	return handler
}

type ApiSessionHandler struct {
	baseHandler
}

func (handler *ApiSessionHandler) newModelEntity() boltEntitySink {
	return &ApiSession{}
}

func (handler *ApiSessionHandler) Create(entity *ApiSession) (string, error) {
	entity.Id = cuid.New() //use cuids which are longer than shortids but are monotonic
	return handler.createEntity(entity)
}

func (handler *ApiSessionHandler) Read(id string) (*ApiSession, error) {
	modelApiSession := &ApiSession{}
	if err := handler.readEntity(id, modelApiSession); err != nil {
		return nil, err
	}
	return modelApiSession, nil
}

func (handler *ApiSessionHandler) ReadByToken(token string) (*ApiSession, error) {
	modelApiSession := &ApiSession{}
	tokenIndex := handler.env.GetStores().ApiSession.GetTokenIndex()
	if err := handler.readEntityWithIndex("token", []byte(token), tokenIndex, modelApiSession); err != nil {
		return nil, err
	}
	return modelApiSession, nil
}

func (handler *ApiSessionHandler) readInTx(tx *bbolt.Tx, id string) (*ApiSession, error) {
	modelApiSession := &ApiSession{}
	if err := handler.readEntityInTx(tx, id, modelApiSession); err != nil {
		return nil, err
	}
	return modelApiSession, nil
}

func (handler *ApiSessionHandler) IsUpdated(_ string) bool {
	return false
}

func (handler *ApiSessionHandler) Update(apiSession *ApiSession) error {
	return handler.updateEntity(apiSession, handler)
}

func (handler *ApiSessionHandler) MfaCompleted(apiSession *ApiSession) error {
	apiSession.MfaComplete = true
	return handler.patchEntity(apiSession, &OrFieldChecker{NewFieldChecker(persistence.FieldAPiSessionMfaComplete), handler})
}

func (handler *ApiSessionHandler) Delete(id string) error {
	return handler.deleteEntity(id)
}

// MarkActivity returns tokens that were not found if any and/or an error.
func (handler *ApiSessionHandler) MarkActivity(tokens []string) ([]string, error) {
	var notFoundTokens []string

	err := handler.GetDb().Batch(func(tx *bbolt.Tx) error {
		store := handler.Store.(persistence.ApiSessionStore)
		mutCtx := boltz.NewMutateContext(tx)
		for _, token := range tokens {
			apiSession, err := store.LoadOneByToken(tx, token)
			if err != nil {
				if boltz.IsErrNotFoundErr(err) {
					notFoundTokens = append(notFoundTokens, token)
				} else {
					return err
				}

			}
			if err = store.Update(mutCtx, apiSession, persistence.UpdateTimeOnlyFieldChecker{}); err != nil {
				if err != nil {
					if boltz.IsErrNotFoundErr(err) {
						notFoundTokens = append(notFoundTokens, token)
					} else {
						return err
					}

				}
			}

			handler.env.GetHandlers().Identity.SetActive(apiSession.IdentityId)
		}
		return nil
	})

	return notFoundTokens, err
}

func (handler *ApiSessionHandler) Stream(query string, collect func(*ApiSession, error) error) error {
	filter, err := ast.Parse(handler.Store, query)

	if err != nil {
		return fmt.Errorf("could not parse query for streaming api sesions: %v", err)
	}

	return handler.env.GetDbProvider().GetDb().View(func(tx *bbolt.Tx) error {
		for cursor := handler.Store.IterateIds(tx, filter); cursor.IsValid(); cursor.Next() {
			current := cursor.Current()

			apiSession, err := handler.readInTx(tx, string(current))
			if err := collect(apiSession, err); err != nil {
				return err
			}
		}
		return collect(nil, nil)
	})
}

func (handler *ApiSessionHandler) StreamIds(query string, collect func(string, error) error) error {
	filter, err := ast.Parse(handler.Store, query)

	if err != nil {
		return fmt.Errorf("could not parse query for streaming api sesions ids: %v", err)
	}

	return handler.env.GetDbProvider().GetDb().View(func(tx *bbolt.Tx) error {
		for cursor := handler.Store.IterateIds(tx, filter); cursor.IsValid(); cursor.Next() {
			current := cursor.Current()
			if err := collect(string(current), err); err != nil {
				return err
			}
		}
		return nil
	})
}

func (handler *ApiSessionHandler) Query(query string) (*ApiSessionListResult, error) {
	result := &ApiSessionListResult{handler: handler}
	err := handler.list(query, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *ApiSessionHandler) VisitFingerprintsForApiSessionId(apiSessionId string, visitor func(fingerprint string) bool) error {
	apiSession, err := handler.Read(apiSessionId)

	if err != nil {
		return errors.Wrapf(err, "could not query fingerprints by api session id [%s]", apiSessionId)
	}

	return handler.VisitFingerprintsForApiSession(apiSession.IdentityId, apiSessionId, visitor)
}

func (handler *ApiSessionHandler) VisitFingerprintsForApiSession(identityId, apiSessionId string, visitor func(fingerprint string) bool) error {
	if stopVisiting, err := handler.env.GetHandlers().Identity.VisitIdentityAuthenticatorFingerprints(identityId, visitor); stopVisiting || err != nil {
		return err
	}

	apiSessionCerts, err := handler.env.GetHandlers().ApiSessionCertificate.ReadByApiSessionId(apiSessionId)
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

type ApiSessionListResult struct {
	handler     *ApiSessionHandler
	ApiSessions []*ApiSession
	models.QueryMetaData
}

func (result *ApiSessionListResult) collect(tx *bbolt.Tx, ids []string, queryMetaData *models.QueryMetaData) error {
	result.QueryMetaData = *queryMetaData
	for _, key := range ids {
		ApiSession, err := result.handler.readInTx(tx, key)
		if err != nil {
			return err
		}
		result.ApiSessions = append(result.ApiSessions, ApiSession)
	}
	return nil
}
