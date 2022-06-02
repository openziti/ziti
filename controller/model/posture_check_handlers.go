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
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	"go.etcd.io/bbolt"
	"strings"
)

const (
	PostureCheckNoTimeout = int64(-1)
)

func NewPostureCheckHandler(env Env) *PostureCheckHandler {
	handler := &PostureCheckHandler{
		baseEntityManager: newBaseEntityManager(env, env.GetStores().PostureCheck),
	}
	handler.impl = handler
	return handler
}

type PostureCheckHandler struct {
	baseEntityManager
}

func (handler *PostureCheckHandler) newModelEntity() boltEntitySink {
	return &PostureCheck{}
}

func (handler *PostureCheckHandler) Create(postureCheckModel *PostureCheck) (string, error) {
	return handler.createEntity(postureCheckModel)
}

func (handler *PostureCheckHandler) Read(id string) (*PostureCheck, error) {
	modelEntity := &PostureCheck{}
	if err := handler.readEntity(id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *PostureCheckHandler) readInTx(tx *bbolt.Tx, id string) (*PostureCheck, error) {
	modelEntity := &PostureCheck{}
	if err := handler.readEntityInTx(tx, id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *PostureCheckHandler) IsUpdated(field string) bool {
	return strings.EqualFold(field, persistence.FieldName) ||
		strings.EqualFold(field, boltz.FieldTags) ||
		strings.EqualFold(field, persistence.FieldRoleAttributes) ||
		strings.EqualFold(field, persistence.FieldPostureCheckOsType) ||
		strings.EqualFold(field, persistence.FieldPostureCheckOsVersions) ||
		strings.EqualFold(field, persistence.FieldPostureCheckMacAddresses) ||
		strings.EqualFold(field, persistence.FieldPostureCheckDomains) ||
		strings.EqualFold(field, persistence.FieldPostureCheckProcessFingerprint) ||
		strings.EqualFold(field, persistence.FieldPostureCheckProcessOs) ||
		strings.EqualFold(field, persistence.FieldPostureCheckProcessPath) ||
		strings.EqualFold(field, persistence.FieldPostureCheckProcessHashes) ||
		strings.EqualFold(field, persistence.FieldPostureCheckMfaPromptOnWake) ||
		strings.EqualFold(field, persistence.FieldPostureCheckMfaPromptOnUnlock) ||
		strings.EqualFold(field, persistence.FieldPostureCheckMfaTimeoutSeconds) ||
		strings.EqualFold(field, persistence.FieldPostureCheckMfaIgnoreLegacyEndpoints) ||
		strings.EqualFold(field, persistence.FieldPostureCheckProcessMultiOsType) ||
		strings.EqualFold(field, persistence.FieldPostureCheckProcessMultiHashes) ||
		strings.EqualFold(field, persistence.FieldPostureCheckProcessMultiPath) ||
		strings.EqualFold(field, persistence.FieldPostureCheckProcessMultiSignerFingerprints) ||
		strings.EqualFold(field, persistence.FieldPostureCheckProcessMultiProcesses) ||
		strings.EqualFold(field, persistence.FieldSemantic)
}

func (handler *PostureCheckHandler) Update(ca *PostureCheck) error {
	return handler.updateEntity(ca, handler)
}

func (handler *PostureCheckHandler) Patch(ca *PostureCheck, checker boltz.FieldChecker) error {
	combinedChecker := &AndFieldChecker{first: handler, second: checker}
	return handler.patchEntity(ca, combinedChecker)
}

func (handler *PostureCheckHandler) Delete(id string) error {
	return handler.deleteEntity(id)
}

func (handler *PostureCheckHandler) Query(query string) (*PostureCheckListResult, error) {
	result := &PostureCheckListResult{handler: handler}
	if err := handler.list(query, result.collect); err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *PostureCheckHandler) QueryPostureChecks(query ast.Query) (*PostureCheckListResult, error) {
	result := &PostureCheckListResult{handler: handler}
	err := handler.preparedList(query, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

type PostureCheckListResult struct {
	handler       *PostureCheckHandler
	PostureChecks []*PostureCheck
	models.QueryMetaData
}

func (result *PostureCheckListResult) collect(tx *bbolt.Tx, ids []string, queryMetaData *models.QueryMetaData) error {
	result.QueryMetaData = *queryMetaData
	for _, key := range ids {
		entity, err := result.handler.readInTx(tx, key)
		if err != nil {
			return err
		}
		result.PostureChecks = append(result.PostureChecks, entity)
	}
	return nil
}
