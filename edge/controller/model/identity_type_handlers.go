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
	"github.com/netfoundry/ziti-edge/edge/controller/util"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"go.etcd.io/bbolt"
)

func NewIdentityTypeHandler(env Env) *IdentityTypeHandler {
	handler := &IdentityTypeHandler{
		baseHandler: baseHandler{
			env:   env,
			store: env.GetStores().IdentityType,
		},
	}
	handler.impl = handler
	return handler
}

type IdentityTypeHandler struct {
	baseHandler
}

func (handler *IdentityTypeHandler) NewModelEntity() BaseModelEntity {
	return &IdentityType{}
}

func (handler *IdentityTypeHandler) HandleCreate(IdentityTypeModel *IdentityType) (string, error) {
	return handler.create(IdentityTypeModel, nil)
}

func (handler *IdentityTypeHandler) HandleRead(id string) (*IdentityType, error) {
	modelEntity := &IdentityType{}
	if err := handler.read(id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *IdentityTypeHandler) HandleReadByIdOrName(idOrName string) (*IdentityType, error) {
	modelEntity := &IdentityType{}
	err := handler.read(idOrName, modelEntity)

	if err == nil {
		return modelEntity, nil
	}

	if !util.IsErrNotFoundErr(err) {
		return nil, err
	}

	if modelEntity.Id == "" {
		modelEntity, err = handler.HandleReadByName(idOrName)
	}

	if err != nil {
		return nil, err
	}

	return modelEntity, nil
}

func (handler *IdentityTypeHandler) handleReadInTx(tx *bbolt.Tx, id string) (*IdentityType, error) {
	modelEntity := &IdentityType{}
	if err := handler.readInTx(tx, id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *IdentityTypeHandler) HandleUpdate(IdentityType *IdentityType) error {
	return handler.update(IdentityType, nil, nil)
}

func (handler *IdentityTypeHandler) HandlePatch(IdentityType *IdentityType, checker boltz.FieldChecker) error {
	return handler.patch(IdentityType, checker, nil)
}

func (handler *IdentityTypeHandler) HandleDelete(id string) error {
	return handler.delete(id, nil, nil)
}

func (handler *IdentityTypeHandler) HandleQuery(query string) (*IdentityTypeListResult, error) {
	result := &IdentityTypeListResult{handler: handler}
	err := handler.list(query, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *IdentityTypeHandler) HandleList(queryOptions *QueryOptions) (*IdentityTypeListResult, error) {
	result := &IdentityTypeListResult{handler: handler}
	err := handler.parseAndList(queryOptions, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *IdentityTypeHandler) HandleReadByName(name string) (*IdentityType, error) {
	modelIdentityType := &IdentityType{}
	nameIndex := handler.env.GetStores().IdentityType.GetNameIndex()
	if err := handler.readWithIndex("name", []byte(name), nameIndex, modelIdentityType); err != nil {
		return nil, err
	}
	return modelIdentityType, nil
}

type IdentityTypeListResult struct {
	handler       *IdentityTypeHandler
	IdentityTypes []*IdentityType
	QueryMetaData
}

func (result *IdentityTypeListResult) collect(tx *bbolt.Tx, ids []string, queryMetaData *QueryMetaData) error {
	result.QueryMetaData = *queryMetaData
	for _, key := range ids {
		entity, err := result.handler.handleReadInTx(tx, key)
		if err != nil {
			return err
		}
		result.IdentityTypes = append(result.IdentityTypes, entity)
	}
	return nil
}
