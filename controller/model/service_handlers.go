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

func (handler *ServiceHandler) Create(service *Service) (string, error) {
	return handler.createEntity(service)
}

func (handler *ServiceHandler) Read(id string) (*Service, error) {
	entity := &Service{}
	if err := handler.readEntity(id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (handler *ServiceHandler) readInTx(tx *bbolt.Tx, id string) (*Service, error) {
	entity := &Service{}
	if err := handler.readEntityInTx(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (handler *ServiceHandler) ReadForIdentity(id string, identityId string) (*Service, error) {
	var service *Service
	err := handler.GetDb().View(func(tx *bbolt.Tx) error {
		identity, err := handler.GetEnv().GetHandlers().Identity.readInTx(tx, identityId)
		if err != nil {
			return err
		}
		if identity.IsAdmin {
			service, err = handler.readInTx(tx, id)
			if err == nil && service != nil {
				service.Permissions = []string{persistence.PolicyTypeBindName, persistence.PolicyTypeDialName}
			}
		} else {
			service, err = handler.ReadForIdentityInTx(tx, id, identityId)
		}
		return err
	})
	return service, err
}

func (handler *ServiceHandler) ReadForIdentityInTx(tx *bbolt.Tx, id string, identityId string) (*Service, error) {
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

func (handler *ServiceHandler) Delete(id string) error {
	return handler.deleteEntity(id, nil)
}

func (handler *ServiceHandler) Update(service *Service) error {
	return handler.updateEntity(service, nil)
}

func (handler *ServiceHandler) Patch(service *Service, checker boltz.FieldChecker) error {
	return handler.patchEntity(service, checker)
}

func (handler *ServiceHandler) GetConfigMap(configTypes map[string]struct{}, service *Service) (map[string]map[string]interface{}, error) {
	configMap := map[string]map[string]interface{}{}
	if len(configTypes) > 0 && len(service.Configs) > 0 {
		configStore := handler.env.GetStores().Config
		configTypeStore := handler.env.GetStores().ConfigType
		err := handler.GetDb().View(func(tx *bbolt.Tx) error {
			_, wantsAll := configTypes["all"]
			for _, configId := range service.Configs {
				config, _ := configStore.LoadOneById(tx, configId)
				if config != nil {
					_, wantsConfig := configTypes[config.Type]
					if wantsAll || wantsConfig {
						configType, _ := configTypeStore.LoadOneById(tx, config.Type)
						if configType != nil {
							configMap[configType.Name] = config.Data
						}
					}
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return configMap, nil
}

func (handler *ServiceHandler) PublicQueryForIdentity(sessionIdentity *Identity, queryOptions *QueryOptions) (*ServiceListResult, error) {
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

func (handler *ServiceHandler) CollectEdgeRouters(id string, collector func(entity BaseModelEntity)) error {
	return handler.collectAssociated(id, persistence.EntityTypeEdgeRouters, handler.env.GetHandlers().EdgeRouter, collector)
}

func (handler *ServiceHandler) CollectServicePolicies(id string, collector func(entity BaseModelEntity)) error {
	return handler.collectAssociated(id, persistence.EntityTypeServicePolicies, handler.env.GetHandlers().ServicePolicy, collector)
}

func (handler *ServiceHandler) CollectConfigs(id string, collector func(entity BaseModelEntity)) error {
	return handler.collectAssociated(id, persistence.EntityTypeConfigs, handler.env.GetHandlers().Config, collector)
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
			service, err = result.handler.ReadForIdentityInTx(tx, key, *result.identityId)
		} else {
			service, err = result.handler.readInTx(tx, key)
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
