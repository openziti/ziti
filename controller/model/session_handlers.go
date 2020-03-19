/*
	Copyright 2019 NetFoundry, Inc.

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
	"github.com/netfoundry/ziti-fabric/controller/models"
	"github.com/netfoundry/ziti-foundation/storage/ast"
	"github.com/netfoundry/ziti-foundation/storage/boltz"

	"go.etcd.io/bbolt"
)

func NewSessionHandler(env Env) *SessionHandler {
	handler := &SessionHandler{
		baseHandler: newBaseHandler(env, env.GetStores().Session),
	}
	handler.impl = handler
	return handler
}

type SessionHandler struct {
	baseHandler
}

func (handler *SessionHandler) newModelEntity() boltEntitySink {
	return &Session{}
}

func (handler *SessionHandler) Create(entity *Session) (string, error) {
	return handler.createEntity(entity)
}

func (handler *SessionHandler) ReadForIdentity(id string, identityId string) (*Session, error) {
	identity, err := handler.GetEnv().GetHandlers().Identity.Read(identityId)

	if err != nil {
		return nil, err
	}
	if identity.IsAdmin {
		return handler.Read(id)
	}

	query := fmt.Sprintf(`id = "%v" and apiSession.identity = "%v"`, id, identityId)
	result, err := handler.Query(query)
	if err != nil {
		return nil, err
	}
	if len(result.Sessions) == 0 {
		return nil, boltz.NewNotFoundError(handler.GetStore().GetSingularEntityType(), "id", id)
	}
	return result.Sessions[0], nil
}

func (handler *SessionHandler) Read(id string) (*Session, error) {
	entity := &Session{}
	if err := handler.readEntity(id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (handler *SessionHandler) readInTx(tx *bbolt.Tx, id string) (*Session, error) {
	entity := &Session{}
	if err := handler.readEntityInTx(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (handler *SessionHandler) DeleteForIdentity(id, identityId string) error {
	session, err := handler.ReadForIdentity(id, identityId)
	if err != nil {
		return err
	}
	if session == nil {
		return boltz.NewNotFoundError(handler.GetStore().GetSingularEntityType(), "id", id)
	}
	return handler.deleteEntity(id)
}

func (handler *SessionHandler) Delete(id string) error {
	return handler.deleteEntity(id)
}

func (handler *SessionHandler) PublicQueryForIdentity(sessionIdentity *Identity, query ast.Query) (*SessionListResult, error) {
	if sessionIdentity.IsAdmin {
		return handler.querySessions(query)
	}
	identityFilterString := fmt.Sprintf(`apiSession.identity = "%v"`, sessionIdentity.Id)
	identityFilter, err := ast.Parse(handler.Store, identityFilterString)
	if err != nil {
		return nil, err
	}
	query.SetPredicate(ast.NewAndExprNode(query.GetPredicate(), identityFilter))
	return handler.querySessions(query)
}

func (handler *SessionHandler) ReadSessionCerts(sessionId string) ([]*SessionCert, error) {
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

func (handler *SessionHandler) Query(query string) (*SessionListResult, error) {
	result := &SessionListResult{handler: handler}
	err := handler.list(query, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *SessionHandler) querySessions(query ast.Query) (*SessionListResult, error) {
	result := &SessionListResult{handler: handler}
	err := handler.preparedList(query, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *SessionHandler) ListSessionsForEdgeRouter(edgeRouterId string) (*SessionListResult, error) {
	result := &SessionListResult{handler: handler}
	query := fmt.Sprintf(`anyOf(apiSession.identity.edgeRouterPolicies.edgeRouters) = "%v" and `+
		`anyOf(service.serviceEdgeRouterPolicies.edgeRouters) = "%v"`, edgeRouterId, edgeRouterId)
	err := handler.list(query, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

type SessionListResult struct {
	handler  *SessionHandler
	Sessions []*Session
	models.QueryMetaData
}

func (result *SessionListResult) collect(tx *bbolt.Tx, ids []string, queryMetaData *models.QueryMetaData) error {
	result.QueryMetaData = *queryMetaData
	for _, key := range ids {
		entity, err := result.handler.readInTx(tx, key)
		if err != nil {
			return err
		}
		result.Sessions = append(result.Sessions, entity)
	}
	return nil
}
