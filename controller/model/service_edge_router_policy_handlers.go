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
)

func NewServiceEdgeRouterPolicyHandler(env Env) *ServiceEdgeRouterPolicyHandler {
	handler := &ServiceEdgeRouterPolicyHandler{
		baseHandler: baseHandler{
			env:   env,
			store: env.GetStores().ServiceEdgeRouterPolicy,
		},
	}
	handler.impl = handler
	return handler
}

type ServiceEdgeRouterPolicyHandler struct {
	baseHandler
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
	return handler.deleteEntity(id, nil)
}

func (handler *ServiceEdgeRouterPolicyHandler) List(queryOptions *QueryOptions) (*ServiceEdgeRouterPolicyListResult, error) {
	result := &ServiceEdgeRouterPolicyListResult{handler: handler}
	err := handler.parseAndList(queryOptions, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *ServiceEdgeRouterPolicyHandler) ListEdgeRouters(id string) ([]*EdgeRouter, error) {
	var result []*EdgeRouter
	err := handler.CollectEdgeRouters(id, func(entity BaseModelEntity) {
		result = append(result, entity.(*EdgeRouter))
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *ServiceEdgeRouterPolicyHandler) CollectEdgeRouters(id string, collector func(entity BaseModelEntity)) error {
	return handler.collectAssociated(id, persistence.EntityTypeEdgeRouters, handler.env.GetHandlers().EdgeRouter, collector)
}

func (handler *ServiceEdgeRouterPolicyHandler) CollectServices(id string, collector func(entity BaseModelEntity)) error {
	return handler.collectAssociated(id, persistence.EntityTypeServices, handler.env.GetHandlers().Service, collector)
}

type ServiceEdgeRouterPolicyListResult struct {
	handler                   *ServiceEdgeRouterPolicyHandler
	ServiceEdgeRouterPolicies []*ServiceEdgeRouterPolicy
	QueryMetaData
}

func (result *ServiceEdgeRouterPolicyListResult) collect(tx *bbolt.Tx, ids []string, queryMetaData *QueryMetaData) error {
	result.QueryMetaData = *queryMetaData
	for _, key := range ids {
		entity, err := result.handler.readInTx(tx, key)
		if err != nil {
			return err
		}
		result.ServiceEdgeRouterPolicies = append(result.ServiceEdgeRouterPolicies, entity)
	}
	return nil
}
