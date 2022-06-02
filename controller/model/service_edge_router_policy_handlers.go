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

func NewServiceEdgeRouterPolicyHandler(env Env) *ServiceEdgeRouterPolicyHandler {
	handler := &ServiceEdgeRouterPolicyHandler{
		baseEntityManager: newBaseEntityManager(env, env.GetStores().ServiceEdgeRouterPolicy),
	}
	handler.impl = handler
	return handler
}

type ServiceEdgeRouterPolicyHandler struct {
	baseEntityManager
}

func (handler *ServiceEdgeRouterPolicyHandler) newModelEntity() boltEntitySink {
	return &ServiceEdgeRouterPolicy{}
}

func (handler *ServiceEdgeRouterPolicyHandler) Create(edgeRouterPolicy *ServiceEdgeRouterPolicy) (string, error) {
	return handler.createEntity(edgeRouterPolicy)
}

func (handler *ServiceEdgeRouterPolicyHandler) Read(id string) (*ServiceEdgeRouterPolicy, error) {
	modelEntity := &ServiceEdgeRouterPolicy{}
	if err := handler.readEntity(id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *ServiceEdgeRouterPolicyHandler) readInTx(tx *bbolt.Tx, id string) (*ServiceEdgeRouterPolicy, error) {
	modelEntity := &ServiceEdgeRouterPolicy{}
	if err := handler.readEntityInTx(tx, id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *ServiceEdgeRouterPolicyHandler) Update(edgeRouterPolicy *ServiceEdgeRouterPolicy) error {
	return handler.updateEntity(edgeRouterPolicy, nil)
}

func (handler *ServiceEdgeRouterPolicyHandler) Patch(edgeRouterPolicy *ServiceEdgeRouterPolicy, checker boltz.FieldChecker) error {
	return handler.patchEntity(edgeRouterPolicy, checker)
}

func (handler *ServiceEdgeRouterPolicyHandler) Delete(id string) error {
	return handler.deleteEntity(id)
}

type ServiceEdgeRouterPolicyListResult struct {
	ServiceEdgeRouterPolicies []*ServiceEdgeRouterPolicy
	models.QueryMetaData
}
