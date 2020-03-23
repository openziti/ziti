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
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/persistence"
	"github.com/netfoundry/ziti-fabric/controller/models"
	"github.com/netfoundry/ziti-foundation/storage/ast"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"go.etcd.io/bbolt"
)

func NewEdgeServiceHandler(env Env) *EdgeServiceHandler {
	handler := &EdgeServiceHandler{
		baseHandler: newBaseHandler(env, env.GetStores().EdgeService),
	}
	handler.impl = handler
	return handler
}

type EdgeServiceHandler struct {
	baseHandler
}

func (handler *EdgeServiceHandler) newModelEntity() boltEntitySink {
	return &ServiceDetail{}
}

func (handler *EdgeServiceHandler) Create(service *Service) (string, error) {
	return handler.createEntity(service)
}

func (handler *EdgeServiceHandler) Read(id string) (*Service, error) {
	entity := &Service{}
	if err := handler.readEntity(id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (handler *EdgeServiceHandler) readInTx(tx *bbolt.Tx, id string) (*ServiceDetail, error) {
	entity := &ServiceDetail{}
	if err := handler.readEntityInTx(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (handler *EdgeServiceHandler) ReadForIdentity(id string, identityId string, configTypes map[string]struct{}) (*ServiceDetail, error) {
	var service *ServiceDetail
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
		if err == nil && len(configTypes) > 0 {
			identityServiceConfigs := handler.env.GetStores().Identity.LoadServiceConfigsByServiceAndType(tx, identityId, configTypes)
			handler.mergeConfigs(tx, configTypes, service, identityServiceConfigs)
		}
		return err
	})
	return service, err
}

func (handler *EdgeServiceHandler) ReadForIdentityInTx(tx *bbolt.Tx, id string, identityId string) (*ServiceDetail, error) {
	query := `id = "%v" and not isEmpty(from servicePolicies where (type = %v and anyOf(identities.id) = "%v"))`

	dialQuery := fmt.Sprintf(query, id, persistence.PolicyTypeDial, identityId)
	_, dialCount, err := handler.GetStore().QueryIds(tx, dialQuery)
	if err != nil {
		return nil, err
	}

	bindQuery := fmt.Sprintf(query, id, persistence.PolicyTypeBind, identityId)
	_, bindCount, err := handler.GetStore().QueryIds(tx, bindQuery)
	if err != nil {
		return nil, err
	}
	result, err := handler.readInTx(tx, id)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, boltz.NewNotFoundError(handler.GetStore().GetSingularEntityType(), "id", id)
	}
	if bindCount > 0 {
		result.Permissions = append(result.Permissions, persistence.PolicyTypeBindName)
	} else if dialCount == 0 {
		return nil, boltz.NewNotFoundError(handler.GetStore().GetSingularEntityType(), "id", id)
	}

	if dialCount > 0 {
		result.Permissions = append(result.Permissions, persistence.PolicyTypeDialName)
	}
	return result, nil
}

func (handler *EdgeServiceHandler) Delete(id string) error {
	return handler.deleteEntity(id)
}

func (handler *EdgeServiceHandler) Update(service *Service) error {
	return handler.updateEntity(service, nil)
}

func (handler *EdgeServiceHandler) Patch(service *Service, checker boltz.FieldChecker) error {
	return handler.patchEntity(service, checker)
}

func (handler *EdgeServiceHandler) PublicQueryForIdentity(sessionIdentity *Identity, configTypes map[string]struct{}, query ast.Query) (*ServiceListResult, error) {
	if sessionIdentity.IsAdmin {
		return handler.queryServices(query, sessionIdentity.Id, configTypes, true)
	}
	idFilterQueryString := fmt.Sprintf(`not isEmpty(from servicePolicies where anyOf(identities) = "%v")`, sessionIdentity.Id)
	idFilterQuery, err := ast.Parse(handler.Store, idFilterQueryString)
	if err != nil {
		return nil, err
	}

	query.SetPredicate(ast.NewAndExprNode(query.GetPredicate(), idFilterQuery.GetPredicate()))

	return handler.queryServices(query, sessionIdentity.Id, configTypes, false)
}

func (handler *EdgeServiceHandler) queryServices(query ast.Query, identityId string, configTypes map[string]struct{}, isAdmin bool) (*ServiceListResult, error) {
	result := &ServiceListResult{
		handler:     handler,
		identityId:  identityId,
		configTypes: configTypes,
		isAdmin:     isAdmin,
	}
	err := handler.preparedList(query, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *EdgeServiceHandler) QueryRoleAttributes(queryString string) ([]string, *models.QueryMetaData, error) {
	index := handler.env.GetStores().EdgeService.GetRoleAttributesIndex()
	return handler.queryRoleAttributes(index, queryString)
}

type ServiceListResult struct {
	handler     *EdgeServiceHandler
	Services    []*ServiceDetail
	identityId  string
	configTypes map[string]struct{}
	isAdmin     bool
	models.QueryMetaData
}

func (result *ServiceListResult) collect(tx *bbolt.Tx, ids []string, queryMetaData *models.QueryMetaData) error {
	result.QueryMetaData = *queryMetaData
	var service *ServiceDetail
	var err error

	identityServiceConfigs := result.handler.env.GetStores().Identity.LoadServiceConfigsByServiceAndType(tx, result.identityId, result.configTypes)

	for _, key := range ids {
		if !result.isAdmin && result.identityId != "" {
			service, err = result.handler.ReadForIdentityInTx(tx, key, result.identityId)
		} else {
			service, err = result.handler.readInTx(tx, key)
			if service != nil && result.isAdmin {
				service.Permissions = []string{persistence.PolicyTypeBindName, persistence.PolicyTypeDialName}
			}
		}
		if err != nil {
			return err
		}
		result.handler.mergeConfigs(tx, result.configTypes, service, identityServiceConfigs)
		result.Services = append(result.Services, service)
	}
	return nil
}

func (handler *EdgeServiceHandler) mergeConfigs(tx *bbolt.Tx, configTypes map[string]struct{}, service *ServiceDetail,
	identityServiceConfigs map[string]map[string]map[string]interface{}) {
	service.Config = map[string]map[string]interface{}{}

	_, wantsAll := configTypes["all"]

	configTypeStore := handler.env.GetStores().ConfigType

	if len(configTypes) > 0 && len(service.Configs) > 0 {
		configStore := handler.env.GetStores().Config
		for _, configId := range service.Configs {
			config, _ := configStore.LoadOneById(tx, configId)
			if config != nil {
				_, wantsConfig := configTypes[config.Type]
				if wantsAll || wantsConfig {
					service.Config[config.Type] = config.Data
				}
			}
		}
	}

	// inject overrides
	if serviceMap, ok := identityServiceConfigs[service.Id]; ok {
		for configTypeId, config := range serviceMap {
			wantsConfig := wantsAll
			if !wantsConfig {
				_, wantsConfig = configTypes[configTypeId]
			}
			if wantsConfig {
				service.Config[configTypeId] = config
			}
		}
	}

	for configTypeId, config := range service.Config {
		configTypeName := configTypeStore.GetName(tx, configTypeId)
		if configTypeName != nil {
			delete(service.Config, configTypeId)
			service.Config[*configTypeName] = config
		} else {
			pfxlog.Logger().Errorf("name for config type %v not found!", configTypeId)
		}
	}
}
