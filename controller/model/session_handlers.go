/*
	Copyright 2019 Netfoundry, Inc.

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

	"github.com/netfoundry/ziti-edge/controller/util"
	"go.etcd.io/bbolt"
)

func NewSessionHandler(env Env) *SessionHandler {
	handler := &SessionHandler{
		baseHandler: baseHandler{
			env:   env,
			store: env.GetStores().Session,
		},
	}
	handler.impl = handler
	return handler
}

type SessionHandler struct {
	baseHandler
}

func (handler *SessionHandler) NewModelEntity() BaseModelEntity {
	return &Session{}
}

func (handler *SessionHandler) HandleCreate(entity *Session) (string, error) {
	return handler.create(entity, nil)
}

func (handler *SessionHandler) HandleReadForIdentity(id string, identityId string) (*Session, error) {
	identity, err := handler.GetEnv().GetHandlers().Identity.HandleRead(identityId)
	if err != nil {
		return nil, err
	}
	if identity.IsAdmin {
		return handler.HandleRead(id)
	}

	query := fmt.Sprintf(`id = "%v" and apiSession.identity = "%v"`, id, identityId)
	result, err := handler.HandleQuery(query)
	if err != nil {
		return nil, err
	}
	if len(result.Sessions) == 0 {
		return nil, util.NewNotFoundError(handler.store.GetSingularEntityType(), "id", id)
	}
	return result.Sessions[0], nil
}

func (handler *SessionHandler) HandleRead(id string) (*Session, error) {
	entity := &Session{}
	if err := handler.read(id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (handler *SessionHandler) handleReadInTx(tx *bbolt.Tx, id string) (*Session, error) {
	entity := &Session{}
	if err := handler.readInTx(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (handler *SessionHandler) HandleDeleteForIdentity(id, identityId string) error {
	session, err := handler.HandleReadForIdentity(id, identityId)
	if err != nil {
		return err
	}
	if session == nil {
		return util.NewNotFoundError(handler.store.GetSingularEntityType(), "id", id)
	}
	return handler.delete(id, nil, nil)
}

func (handler *SessionHandler) HandleDelete(id string) error {
	return handler.delete(id, nil, nil)
}

func (handler *SessionHandler) HandleListForIdentity(sessionIdentity *Identity, queryOptions *QueryOptions) (*SessionListResult, error) {
	if sessionIdentity.IsAdmin {
		return handler.HandleList(queryOptions)
	}
	query := queryOptions.Predicate
	// TODO: Convert model errors to appropriate api errors
	if query != "" {
		query = "(" + query + ") and "
	}
	query += fmt.Sprintf(`apiSession.identity = "%v"`, sessionIdentity.Id)
	queryOptions.finalQuery = query
	return handler.HandleList(queryOptions)
}

func (handler *SessionHandler) HandleReadSessionCerts(sessionId string) ([]*SessionCert, error) {
	var result []*SessionCert
	err := handler.GetDb().View(func(tx *bbolt.Tx) error {
		var err error
		certs, err := handler.GetEnv().GetStores().Session.LoadCerts(tx, sessionId)
		if err != nil {
			return err
		}
		for _, cert := range certs {
			modelSessionCert := &SessionCert{}
			if err = modelSessionCert.FillFrom(handler, tx, cert); err != nil {
				return err
			}
			result = append(result, modelSessionCert)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *SessionHandler) HandleQuery(query string) (*SessionListResult, error) {
	result := &SessionListResult{handler: handler}
	err := handler.list(query, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *SessionHandler) HandleList(queryOptions *QueryOptions) (*SessionListResult, error) {
	result := &SessionListResult{handler: handler}
	err := handler.parseAndList(queryOptions, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *SessionHandler) HandleListSessionForEdgeRouter(edgeRouterId string) (*SessionListResult, error) {
	result := &SessionListResult{handler: handler}
	query := fmt.Sprintf(`anyOf(apiSession.identity.edgeRouterPolicies.edgeRouters) = "%v" and `+
		`(isEmpty(service.edgeRouterRoles) or (anyOf(service.edgeRouters) = "%v"))`, edgeRouterId, edgeRouterId)
	err := handler.list(query, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

type SessionListResult struct {
	handler  *SessionHandler
	Sessions []*Session
	QueryMetaData
}

func (result *SessionListResult) collect(tx *bbolt.Tx, ids []string, queryMetaData *QueryMetaData) error {
	result.QueryMetaData = *queryMetaData
	for _, key := range ids {
		entity, err := result.handler.handleReadInTx(tx, key)
		if err != nil {
			return err
		}
		result.Sessions = append(result.Sessions, entity)
	}
	return nil
}
