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
	"github.com/netfoundry/ziti-edge/controller/persistence"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"go.etcd.io/bbolt"
	"strings"
)

func NewCaHandler(env Env) *CaHandler {
	handler := &CaHandler{
		baseHandler: baseHandler{
			env:   env,
			store: env.GetStores().Ca,
		},
	}
	handler.impl = handler
	return handler
}

type CaHandler struct {
	baseHandler
}

func (handler *CaHandler) NewModelEntity() BaseModelEntity {
	return &Ca{}
}

func (handler *CaHandler) Create(caModel *Ca) (string, error) {
	return handler.createEntity(caModel)
}

func (handler *CaHandler) Read(id string) (*Ca, error) {
	modelEntity := &Ca{}
	if err := handler.readEntity(id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *CaHandler) readInTx(tx *bbolt.Tx, id string) (*Ca, error) {
	modelEntity := &Ca{}
	if err := handler.readEntityInTx(tx, id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *CaHandler) IsUpdated(field string) bool {
	return strings.EqualFold(field, persistence.FieldName) ||
		strings.EqualFold(field, persistence.FieldTags) ||
		strings.EqualFold(field, persistence.FieldCaIsAutoCaEnrollmentEnabled) ||
		strings.EqualFold(field, persistence.FieldCaIsOttCaEnrollmentEnabled) ||
		strings.EqualFold(field, persistence.FieldCaIsAuthEnabled)
}

func (handler *CaHandler) Update(ca *Ca) error {
	return handler.updateEntity(ca, handler)
}

func (handler *CaHandler) Patch(ca *Ca, checker boltz.FieldChecker) error {
	combinedChecker := &AndFieldChecker{first: handler, second: checker}
	return handler.patchEntity(ca, combinedChecker)
}

func (handler *CaHandler) Verified(ca *Ca) error {
	ca.IsVerified = true
	checker := &boltz.MapFieldChecker{
		persistence.FieldCaIsVerified: struct{}{},
	}
	return handler.patchEntity(ca, checker)
}

func (handler *CaHandler) Delete(id string) error {
	return handler.deleteEntity(id, nil)
}

func (handler *CaHandler) Query(query string) (*CaListResult, error) {
	result := &CaListResult{handler: handler}
	if err := handler.list(query, result.collect); err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *CaHandler) PublicQuery(queryOptions *QueryOptions) (*CaListResult, error) {
	result := &CaListResult{handler: handler}
	if err := handler.parseAndList(queryOptions, result.collect); err != nil {
		return nil, err
	}
	return result, nil
}

type CaListResult struct {
	handler *CaHandler
	Cas     []*Ca
	QueryMetaData
}

func (result *CaListResult) collect(tx *bbolt.Tx, ids []string, queryMetaData *QueryMetaData) error {
	result.QueryMetaData = *queryMetaData
	for _, key := range ids {
		entity, err := result.handler.readInTx(tx, key)
		if err != nil {
			return err
		}
		result.Cas = append(result.Cas, entity)
	}
	return nil
}
