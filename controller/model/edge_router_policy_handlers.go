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
	"github.com/openziti/storage/boltz"
	"go.etcd.io/bbolt"
)

func NewEdgeRouterPolicyHandler(env Env) *EdgeRouterPolicyHandler {
	handler := &EdgeRouterPolicyHandler{
		baseEntityManager: newBaseEntityManager(env, env.GetStores().EdgeRouterPolicy),
	}
	handler.impl = handler
	return handler
}

type EdgeRouterPolicyHandler struct {
	baseEntityManager
}

func (handler *EdgeRouterPolicyHandler) newModelEntity() boltEntitySink {
	return &EdgeRouterPolicy{}
}

func (handler *EdgeRouterPolicyHandler) Create(edgeRouterPolicy *EdgeRouterPolicy) (string, error) {
	return handler.createEntity(edgeRouterPolicy)
}

func (handler *EdgeRouterPolicyHandler) Read(id string) (*EdgeRouterPolicy, error) {
	modelEntity := &EdgeRouterPolicy{}
	if err := handler.readEntity(id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *EdgeRouterPolicyHandler) readInTx(tx *bbolt.Tx, id string) (*EdgeRouterPolicy, error) {
	modelEntity := &EdgeRouterPolicy{}
	if err := handler.readEntityInTx(tx, id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *EdgeRouterPolicyHandler) Update(edgeRouterPolicy *EdgeRouterPolicy) error {
	return handler.updateEntity(edgeRouterPolicy, nil)
}

func (handler *EdgeRouterPolicyHandler) Patch(edgeRouterPolicy *EdgeRouterPolicy, checker boltz.FieldChecker) error {
	return handler.patchEntity(edgeRouterPolicy, checker)
}

func (handler *EdgeRouterPolicyHandler) Delete(id string) error {
	return handler.deleteEntity(id)
}

type EdgeRouterPolicyListResult struct {
	EdgeRouterPolicies []*EdgeRouterPolicy
	models.QueryMetaData
}
