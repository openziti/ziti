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
	"github.com/netfoundry/ziti-edge/edge/controller/persistence"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"go.etcd.io/bbolt"
)

func NewServicePolicyHandler(env Env) *ServicePolicyHandler {
	handler := &ServicePolicyHandler{
		baseHandler: baseHandler{
			env:   env,
			store: env.GetStores().ServicePolicy,
		},
	}
	handler.impl = handler
	return handler
}

type ServicePolicyHandler struct {
	baseHandler
}

func (handler *ServicePolicyHandler) NewModelEntity() BaseModelEntity {
	return &ServicePolicy{}
}

func (handler *ServicePolicyHandler) HandleCreate(servicePolicy *ServicePolicy) (string, error) {
	return handler.create(servicePolicy, nil)
}

func (handler *ServicePolicyHandler) HandleRead(id string) (*ServicePolicy, error) {
	modelEntity := &ServicePolicy{}
	if err := handler.read(id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *ServicePolicyHandler) handleReadInTx(tx *bbolt.Tx, id string) (*ServicePolicy, error) {
	modelEntity := &ServicePolicy{}
	if err := handler.readInTx(tx, id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *ServicePolicyHandler) HandleUpdate(servicePolicy *ServicePolicy) error {
	return handler.update(servicePolicy, nil, nil)
}

func (handler *ServicePolicyHandler) HandlePatch(servicePolicy *ServicePolicy, checker boltz.FieldChecker) error {
	return handler.patch(servicePolicy, checker, nil)
}

func (handler *ServicePolicyHandler) HandleDelete(id string) error {
	return handler.delete(id, nil, nil)
}

func (handler *ServicePolicyHandler) HandleList(queryOptions *QueryOptions) (*ServicePolicyListResult, error) {
	result := &ServicePolicyListResult{handler: handler}
	err := handler.parseAndList(queryOptions, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *ServicePolicyHandler) HandleListServices(id string) ([]*Service, error) {
	var result []*Service
	err := handler.HandleCollectServices(id, func(entity BaseModelEntity) {
		result = append(result, entity.(*Service))
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *ServicePolicyHandler) HandleCollectServices(id string, collector func(entity BaseModelEntity)) error {
	return handler.HandleCollectAssociated(id, persistence.EntityTypeServices, handler.env.GetHandlers().Service, collector)
}

func (handler *ServicePolicyHandler) HandleCollectIdentities(id string, collector func(entity BaseModelEntity)) error {
	return handler.HandleCollectAssociated(id, persistence.EntityTypeIdentities, handler.env.GetHandlers().Identity, collector)
}

type ServicePolicyListResult struct {
	handler         *ServicePolicyHandler
	ServicePolicies []*ServicePolicy
	QueryMetaData
}

func (result *ServicePolicyListResult) collect(tx *bbolt.Tx, ids []string, queryMetaData *QueryMetaData) error {
	result.QueryMetaData = *queryMetaData
	for _, key := range ids {
		entity, err := result.handler.handleReadInTx(tx, key)
		if err != nil {
			return err
		}
		result.ServicePolicies = append(result.ServicePolicies, entity)
	}
	return nil
}
