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
	"strings"

	"github.com/pkg/errors"

	"github.com/netfoundry/ziti-edge/controller/persistence"
	"github.com/netfoundry/ziti-edge/controller/util"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"go.etcd.io/bbolt"
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
	var service *Service
	err := handler.GetDb().View(func(tx *bbolt.Tx) error {
		identity, err := handler.GetEnv().GetHandlers().Identity.handleReadInTx(tx, identityId)
		if err != nil {
			return err
		}
		if identity.IsAdmin {
			service, err = handler.handleReadInTx(tx, id)
			if service != nil {
				service.Permissions = []string{persistence.PolicyTypeBindName, persistence.PolicyTypeDialName}
			}
		} else {
			service, err = handler.HandleReadForIdentityInTx(tx, id, identityId)
		}
		return err
	})
	return service, err
}

func (handler *ServiceHandler) HandleReadForIdentityInTx(tx *bbolt.Tx, id string, identityId string) (*Service, error) {
	query := `id = "%v" and not isEmpty(from servicePolicies where (type = %v and anyOf(identities.id) = "%v"))`

	dialQuery := fmt.Sprintf(query, id, persistence.PolicyTypeDial, identityId)
	dialResult, err := handler.queryServices(tx, dialQuery)
	if err != nil {
		return nil, err
	}

	bindQuery := fmt.Sprintf(query, id, persistence.PolicyTypeBind, identityId)
	bindResult, err := handler.queryServices(tx, bindQuery)
	if err != nil {
		return nil, err
	}
	if len(bindResult.Services) > 1 || len(dialResult.Services) > 1 {
		return nil, errors.Errorf("Got more than one result while checking permissions. dial: %v, bind: %v",
			len(dialResult.Services), len(bindResult.Services))
	}
	var result *Service
	if len(bindResult.Services) == 1 {
		result = bindResult.Services[0]
		result.Permissions = append(result.Permissions, persistence.PolicyTypeBindName)
	}
	if len(dialResult.Services) == 1 {
		if result == nil {
			result = dialResult.Services[0]
		}
		result.Permissions = append(result.Permissions, persistence.PolicyTypeDialName)
	}
	if result == nil {
		return nil, util.NewNotFoundError(handler.store.GetSingularEntityType(), "id", id)
	}
	return result, nil
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

func (handler *ServiceHandler) HandleListForIdentity(sessionIdentity *Identity, queryOptions *QueryOptions) (*ServiceListResult, error) {
	if sessionIdentity.IsAdmin {
		return handler.listServices(queryOptions, nil, true)
	}
	query := queryOptions.Predicate
	if query != "" {
		query = "(" + query + ") and "
	}
	query += fmt.Sprintf(`not isEmpty(from servicePolicies where anyOf(identities) = "%v")`, sessionIdentity.Id)
	queryOptions.finalQuery = query
	return handler.listServices(queryOptions, &sessionIdentity.Id, false)
}

func (handler *ServiceHandler) queryServices(tx *bbolt.Tx, query string) (*ServiceListResult, error) {
	result := &ServiceListResult{handler: handler}
	err := handler.listWithTx(tx, query, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *ServiceHandler) listServices(queryOptions *QueryOptions, identityId *string, isAdmin bool) (*ServiceListResult, error) {
	result := &ServiceListResult{
		handler:    handler,
		identityId: identityId,
		isAdmin:    isAdmin,
	}
	err := handler.parseAndList(queryOptions, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *ServiceHandler) HandleCollectEdgeRouters(id string, collector func(entity BaseModelEntity)) error {
	return handler.HandleCollectAssociated(id, persistence.EntityTypeEdgeRouters, handler.env.GetHandlers().EdgeRouter, collector)
}

func (handler *ServiceHandler) HandleCollectServicePolicies(id string, collector func(entity BaseModelEntity)) error {
	return handler.HandleCollectAssociated(id, persistence.EntityTypeServicePolicies, handler.env.GetHandlers().ServicePolicy, collector)
}

type ServiceListResult struct {
	handler    *ServiceHandler
	Services   []*Service
	identityId *string
	isAdmin    bool
	QueryMetaData
}

func (result *ServiceListResult) collect(tx *bbolt.Tx, ids []string, queryMetaData *QueryMetaData) error {
	result.QueryMetaData = *queryMetaData
	var service *Service
	var err error
	for _, key := range ids {
		if result.identityId != nil {
			service, err = result.handler.HandleReadForIdentityInTx(tx, key, *result.identityId)
		} else {
			service, err = result.handler.handleReadInTx(tx, key)
			if service != nil && result.isAdmin {
				service.Permissions = []string{persistence.PolicyTypeBindName, persistence.PolicyTypeDialName}
			}
		}
		if err != nil {
			return err
		}
		result.Services = append(result.Services, service)
	}
	return nil
}
