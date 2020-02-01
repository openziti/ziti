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
	"go.etcd.io/bbolt"
)

func NewApiSessionHandler(env Env) *ApiSessionHandler {
	handler := &ApiSessionHandler{
		baseHandler: baseHandler{
			env:   env,
			store: env.GetStores().ApiSession,
		},
	}
	handler.impl = handler
	return handler
}

type ApiSessionHandler struct {
	baseHandler
}

func (handler *ApiSessionHandler) NewModelEntity() BaseModelEntity {
	return &ApiSession{}
}

func (handler *ApiSessionHandler) Create(ApiSessionModel *ApiSession) (string, error) {
	return handler.createEntity(ApiSessionModel)
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

func (handler *ApiSessionHandler) Delete(id string) error {
	return handler.deleteEntity(id, nil)
}

func (handler *ApiSessionHandler) MarkActivity(tokens []string) error {
	return handler.GetDb().Update(func(tx *bbolt.Tx) error {
		return handler.GetEnv().GetStores().ApiSession.MarkActivity(tx, tokens)
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

func (handler *ApiSessionHandler) PublicQuery(queryOptions *QueryOptions) (*ApiSessionListResult, error) {
	result := &ApiSessionListResult{handler: handler}
	err := handler.parseAndList(queryOptions, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

type ApiSessionListResult struct {
	handler     *ApiSessionHandler
	ApiSessions []*ApiSession
	QueryMetaData
}

func (result *ApiSessionListResult) collect(tx *bbolt.Tx, ids []string, queryMetaData *QueryMetaData) error {
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
