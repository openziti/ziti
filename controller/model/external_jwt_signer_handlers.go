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
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/storage/ast"
	"github.com/openziti/foundation/storage/boltz"
)

func NewExternalJwtSignerHandler(env Env) *ExternalJwtSignerHandler {
	handler := &ExternalJwtSignerHandler{
		baseHandler: newBaseHandler(env, env.GetStores().ExternalJwtSigner),
	}
	handler.impl = handler
	return handler
}

type ExternalJwtSignerHandler struct {
	baseHandler
}

func (handler *ExternalJwtSignerHandler) IsUpdated(_ string) bool {
	return true
}

func (handler *ExternalJwtSignerHandler) newModelEntity() boltEntitySink {
	return &ExternalJwtSigner{}
}

func (handler *ExternalJwtSignerHandler) Create(ExternalJwtSignerModel *ExternalJwtSigner) (string, error) {
	return handler.createEntity(ExternalJwtSignerModel)
}

func (handler *ExternalJwtSignerHandler) Read(id string) (*ExternalJwtSigner, error) {
	modelEntity := &ExternalJwtSigner{}
	if err := handler.readEntity(id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *ExternalJwtSignerHandler) Update(signer *ExternalJwtSigner) error {
	return handler.updateEntity(signer, handler)
}

func (handler *ExternalJwtSignerHandler) Patch(signer *ExternalJwtSigner, fields boltz.FieldChecker) error {
	return handler.patchEntity(signer, fields)
}

func (handler *ExternalJwtSignerHandler) Delete(id string) error {
	return handler.deleteEntity(id)
}

type ListExtJwtSignerResult struct {
	handler       *ExternalJwtSignerHandler
	QueryMetaData models.QueryMetaData
	ExtJwtSigners []*ExternalJwtSigner
}

func (handler *ExternalJwtSignerHandler) PublicQuery(query ast.Query) (*ListExtJwtSignerResult, error) {
	queryStr := "enabled = true"
	enabledQuery, err := ast.Parse(handler.Store, queryStr)
	if err != nil {
		return nil, err
	}

	query.SetPredicate(ast.NewAndExprNode(query.GetPredicate(), enabledQuery.GetPredicate()))

	entityResult, err := handler.BasePreparedList(query)

	if err != nil {
		return nil, err
	}

	result := &ListExtJwtSignerResult{
		handler:       handler,
		QueryMetaData: entityResult.QueryMetaData,
	}

	for _, entity := range entityResult.Entities {
		if extJwtSigner, ok := entity.(*ExternalJwtSigner); ok {
			result.ExtJwtSigners = append(result.ExtJwtSigners, extJwtSigner)
		}
	}

	return result, nil
}
