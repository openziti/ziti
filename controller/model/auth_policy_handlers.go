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
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/openziti/foundation/util/errorz"
	"go.etcd.io/bbolt"
)

func NewAuthPolicyHandler(env Env) *AuthPolicyHandler {
	handler := &AuthPolicyHandler{
		baseHandler: newBaseHandler(env, env.GetStores().AuthPolicy),
	}
	handler.impl = handler
	return handler
}

type AuthPolicyHandler struct {
	baseHandler
}

func (handler *AuthPolicyHandler) newModelEntity() boltEntitySink {
	return &AuthPolicy{}
}

func (handler *AuthPolicyHandler) verifyExtJwt(id string, fieldName string) error {
	extJwtSigner, err := handler.env.GetHandlers().ExternalJwtSigner.Read(id)

	if err != nil && !boltz.IsErrNotFoundErr(err) {
		return err
	}

	if extJwtSigner == nil {
		apiErr := errorz.NewNotFound()
		apiErr.Cause = errorz.NewFieldError("not found", fieldName, id)
		apiErr.AppendCause = true
		return apiErr
	}

	return nil
}

func (handler *AuthPolicyHandler) Create(authPolicy *AuthPolicy) (string, error) {
	if authPolicy.Secondary.RequiredExtJwtSigner != nil {
		if err := handler.verifyExtJwt(*authPolicy.Secondary.RequiredExtJwtSigner, "secondary.requiredExtJwtSigner"); err != nil {
			return "", err
		}
	}

	for i, extJwtId := range authPolicy.Primary.ExtJwt.AllowedExtJwtSigners {
		if err := handler.verifyExtJwt(extJwtId, fmt.Sprintf("primary.extJwt.allowedExtJwtSigners[%d]", i)); err != nil {
			return "", err
		}
	}

	return handler.createEntity(authPolicy)
}

func (handler *AuthPolicyHandler) Read(id string) (*AuthPolicy, error) {
	modelEntity := &AuthPolicy{}
	if err := handler.readEntity(id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *AuthPolicyHandler) readInTx(tx *bbolt.Tx, id string) (*AuthPolicy, error) {
	modelEntity := &AuthPolicy{}
	if err := handler.readEntityInTx(tx, id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *AuthPolicyHandler) IsUpdated(_ string) bool {
	return true
}

func (handler *AuthPolicyHandler) Update(ca *AuthPolicy) error {
	return handler.updateEntity(ca, handler)
}

func (handler *AuthPolicyHandler) Patch(ca *AuthPolicy, checker boltz.FieldChecker) error {
	return handler.patchEntity(ca, checker)
}

func (handler *AuthPolicyHandler) Delete(id string) error {
	return handler.deleteEntity(id)
}

func (handler *AuthPolicyHandler) Query(query string) (*AuthPolicyListResult, error) {
	result := &AuthPolicyListResult{handler: handler}
	if err := handler.list(query, result.collect); err != nil {
		return nil, err
	}
	return result, nil
}

type AuthPolicyListResult struct {
	handler      *AuthPolicyHandler
	AuthPolicies []*AuthPolicy
	models.QueryMetaData
}

func (result *AuthPolicyListResult) collect(tx *bbolt.Tx, ids []string, queryMetaData *models.QueryMetaData) error {
	result.QueryMetaData = *queryMetaData
	for _, key := range ids {
		authPolicy, err := result.handler.readInTx(tx, key)
		if err != nil {
			return err
		}
		result.AuthPolicies = append(result.AuthPolicies, authPolicy)
	}
	return nil
}
