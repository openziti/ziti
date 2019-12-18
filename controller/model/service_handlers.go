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
	"fmt"
	"github.com/netfoundry/ziti-edge/controller/persistence"
	"github.com/netfoundry/ziti-edge/controller/util"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"go.etcd.io/bbolt"
	"strings"
)

func NewServiceHandler(env Env) *ServiceHandler {
	handler := &ServiceHandler{
		baseHandler: baseHandler{
			env:   env,
			store: env.GetStores().EdgeService,
		},
	}
	handler.impl = handler
	return handler
}

type ServiceHandler struct {
	baseHandler
}

func (handler *ServiceHandler) NewModelEntity() BaseModelEntity {
	return &Service{}
}

func (handler *ServiceHandler) HandleCreate(service *Service) (string, error) {
	return handler.create(service, nil)
}

func (handler *ServiceHandler) HandleRead(id string) (*Service, error) {
	entity := &Service{}
	if err := handler.read(id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (handler *ServiceHandler) handleReadInTx(tx *bbolt.Tx, id string) (*Service, error) {
	entity := &Service{}
	if err := handler.readInTx(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (handler *ServiceHandler) HandleReadForIdentity(id string, identityId string) (*Service, error) {
	identity, err := handler.GetEnv().GetHandlers().Identity.HandleRead(identityId)
	if err != nil {
		return nil, err
	}
	if identity.IsAdmin {
		return handler.HandleRead(id)
	}

	query := fmt.Sprintf(`id = "%v" and anyOf(appwans.identities.id) = "%v"`, id, identityId)
	result, err := handler.HandleQuery(query)
	if err != nil {
		return nil, err
	}
	if len(result.Services) == 0 {
		return nil, util.NewNotFoundError(handler.store.GetSingularEntityType(), "id", id)
	}
	return result.Services[0], nil
}

func (handler *ServiceHandler) HandleDelete(id string) error {
	return handler.delete(id, nil, nil)
}

func (handler *ServiceHandler) IsUpdated(field string) bool {
	return !strings.EqualFold(field, "appwans") &&
		!strings.EqualFold(field, "HostIds") &&
		!strings.EqualFold(field, "Clusters")
}

func (handler *ServiceHandler) HandleUpdate(service *Service) error {
	return handler.update(service, nil, nil)
}

func (handler *ServiceHandler) HandlePatch(service *Service, checker boltz.FieldChecker) error {
	return handler.patch(service, checker, nil)
}

type ServiceListResult struct {
	handler  *ServiceHandler
	Services []*Service
	QueryMetaData
}

func (result *ServiceListResult) collect(tx *bbolt.Tx, ids []string, queryMetaData *QueryMetaData) error {
	result.QueryMetaData = *queryMetaData
	for _, key := range ids {
		service, err := result.handler.handleReadInTx(tx, key)
		if err != nil {
			return err
		}
		result.Services = append(result.Services, service)
	}
	return nil
}

func (handler *ServiceHandler) HandleListForIdentity(sessionIdentity *Identity, queryOptions *QueryOptions) (*ServiceListResult, error) {
	if sessionIdentity.IsAdmin {
		return handler.HandleList(queryOptions)
	}

	query := queryOptions.Predicate
	// TODO: Convert model errors to appropriate api errors
	if query != "" {
		query = "(" + query + ") and "
	}
	query += fmt.Sprintf(`anyOf(appwans.identities.id) = "%v"`, sessionIdentity.Id)
	queryOptions.finalQuery = query
	return handler.HandleList(queryOptions)
}

func (handler *ServiceHandler) HandleQuery(query string) (*ServiceListResult, error) {
	result := &ServiceListResult{handler: handler}
	err := handler.list(query, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *ServiceHandler) HandleList(queryOptions *QueryOptions) (*ServiceListResult, error) {
	result := &ServiceListResult{handler: handler}
	err := handler.parseAndList(queryOptions, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *ServiceHandler) HandleCollectEdgeRouters(id string, collector func(entity BaseModelEntity)) error {
	return handler.HandleCollectAssociated(id, persistence.EntityTypeEdgeRouters, handler.env.GetHandlers().EdgeRouter, collector)
}

func (handler *ServiceHandler) HandleCollectHostIds(id string, collector func(entity BaseModelEntity)) error {
	return handler.HandleCollectAssociated(id, persistence.FieldServiceHostingIdentities, handler.env.GetHandlers().Identity, collector)
}

func (handler *ServiceHandler) HandleCollectServicePolicies(id string, collector func(entity BaseModelEntity)) error {
	return handler.HandleCollectAssociated(id, persistence.EntityTypeServicePolicies, handler.env.GetHandlers().ServicePolicy, collector)
}
