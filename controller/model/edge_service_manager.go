/*
	Copyright NetFoundry Inc.

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
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/v2/common/pb/edge_cmd_pb"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/command"
	"github.com/openziti/ziti/v2/controller/db"
	"github.com/openziti/ziti/v2/controller/fields"
	"github.com/openziti/ziti/v2/controller/models"
	"github.com/openziti/ziti/v2/controller/storage/ast"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"go.etcd.io/bbolt"
	"google.golang.org/protobuf/proto"
)

func NewEdgeServiceManager(env Env) *EdgeServiceManager {
	manager := &EdgeServiceManager{
		baseEntityManager: newBaseEntityManager[*EdgeService, *db.Service](env, env.GetStores().Service),
		detailLister:      &ServiceDetailLister{},
	}
	manager.impl = manager
	manager.detailLister.manager = manager

	RegisterManagerDecoder[*EdgeService](env, manager)

	return manager
}

type EdgeServiceManager struct {
	baseEntityManager[*EdgeService, *db.Service]
	detailLister *ServiceDetailLister
}

func (self *EdgeServiceManager) GetDetailLister() *ServiceDetailLister {
	return self.detailLister
}

func (self *EdgeServiceManager) GetEntityTypeId() string {
	return "edgeServices"
}

func (self *EdgeServiceManager) NewModelEntity() *EdgeService {
	return &EdgeService{}
}

func (self *EdgeServiceManager) Create(entity *EdgeService, ctx *change.Context) error {
	return DispatchCreate[*EdgeService](self, entity, ctx)
}

func (self *EdgeServiceManager) ApplyCreate(cmd *command.CreateEntityCommand[*EdgeService], ctx boltz.MutateContext) error {
	_, err := self.createEntity(cmd.Entity, ctx)
	return err
}

func (self *EdgeServiceManager) Update(entity *EdgeService, checker fields.UpdatedFields, ctx *change.Context) error {
	if checker != nil {
		checker = checker.RemoveFields("encryptionRequired")
	}
	return DispatchUpdate[*EdgeService](self, entity, checker, ctx)
}

func (self *EdgeServiceManager) ApplyUpdate(cmd *command.UpdateEntityCommand[*EdgeService], ctx boltz.MutateContext) error {
	return self.updateEntity(cmd.Entity, cmd.UpdatedFields, ctx)
}

// notFoundIfFabricOnly returns a NotFound error if the given service is fabric-only. Fabric-only
// services are not part of the edge service API surface, so edge-facing single-entity reads and
// deletes must treat them as absent. The list/query paths apply an isFabricOnly = false predicate
// as an optimization; this guard is the authoritative protection for the by-id entry points.
//
// TEMPORARY(fabric-edge-collapse): the fabric/edge service split is scaffolding kept only so a few
// seldom-used maintenance ("fabric-only") services can stay hidden from the edge API. The end goal
// is to erase the distinction entirely. When that happens, remove EdgeServiceManager's fabric-only
// guards (this helper, andNotFabricOnly, and their callers) along with Service.IsFabricOnly. Grep
// for "TEMPORARY(fabric-edge-collapse)" to find every site.
func (self *EdgeServiceManager) notFoundIfFabricOnly(tx *bbolt.Tx, id string) error {
	isFabricOnly := boltz.FieldToBool(self.GetStore().GetSymbol(db.FieldServiceIsFabricOnly).Eval(tx, []byte(id)))
	if isFabricOnly != nil && *isFabricOnly {
		return boltz.NewNotFoundError(self.GetStore().GetSingularEntityType(), "id", id)
	}
	return nil
}

func (self *EdgeServiceManager) Read(id string) (*EdgeService, error) {
	entity := &EdgeService{}
	err := self.GetDb().View(func(tx *bbolt.Tx) error {
		if err := self.notFoundIfFabricOnly(tx, id); err != nil {
			return err
		}
		return self.readEntityInTx(tx, id, entity)
	})
	if err != nil {
		return nil, err
	}
	return entity, nil
}

func (self *EdgeServiceManager) ReadByName(name string) (*EdgeService, error) {
	entity := &EdgeService{}
	nameIndex := self.env.GetStores().Service.GetNameIndex()
	err := self.GetDb().View(func(tx *bbolt.Tx) error {
		id := nameIndex.Read(tx, []byte(name))
		if id == nil {
			return boltz.NewNotFoundError(self.GetStore().GetSingularEntityType(), "name", name)
		}
		if err := self.notFoundIfFabricOnly(tx, string(id)); err != nil {
			return err
		}
		return self.readEntityInTx(tx, string(id), entity)
	})
	if err != nil {
		return nil, err
	}
	return entity, nil
}

func (self *EdgeServiceManager) Delete(id string, ctx *change.Context) error {
	if err := self.GetDb().View(func(tx *bbolt.Tx) error {
		return self.notFoundIfFabricOnly(tx, id)
	}); err != nil {
		return err
	}
	return self.baseEntityManager.Delete(id, ctx)
}

// PreparedListAssociatedWithHandler guards the edge service association routes (e.g.
// /services/{id}/terminators, /service-policies, /service-edge-router-policies, /configs): a
// fabric-only service is not part of the edge API surface, so listing its associations must report
// it absent, consistent with Read/ReadByName/Delete. The base implementation reads related entities
// straight from the unified store without proving the source id is an edge service.
func (self *EdgeServiceManager) PreparedListAssociatedWithHandler(id string, association string, query ast.Query, handler models.ListResultHandler) error {
	return self.GetDb().View(func(tx *bbolt.Tx) error {
		if err := self.notFoundIfFabricOnly(tx, id); err != nil {
			return err
		}
		return self.PreparedListAssociatedWithTx(tx, id, association, query, handler)
	})
}

// readInTx is the shared edge service-detail loader. It is the single point through which both
// ReadForIdentityInTx and the ServiceDetailLister materialize entities, so the fabric-only guard
// lives here: a fabric-only service is not part of the edge API surface and is reported as absent.
func (self *EdgeServiceManager) readInTx(tx *bbolt.Tx, id string) (*ServiceDetail, error) {
	entity := &ServiceDetail{}
	boltEntity := self.GetStore().GetEntityStrategy().NewEntity()
	found, err := self.GetStore().LoadEntity(tx, id, boltEntity)
	if err != nil {
		return nil, err
	}
	if !found || boltEntity.IsFabricOnly {
		return nil, boltz.NewNotFoundError(self.GetStore().GetSingularEntityType(), "id", id)
	}

	if err = entity.fillFrom(self.env, tx, boltEntity); err != nil {
		return nil, err
	}
	return entity, nil
}

// andNotFabricOnly restricts a query to non-fabric-only services. Edge service list paths must keep
// fabric-only services out of their result set so they are never materialized via readInTx (which
// would otherwise fail the whole list).
//
// TEMPORARY(fabric-edge-collapse): remove with the fabric/edge split; see notFoundIfFabricOnly.
func (self *EdgeServiceManager) andNotFabricOnly(query ast.Query) error {
	notFabricOnly, err := ast.Parse(self.GetStore(), "isFabricOnly = false")
	if err != nil {
		return err
	}
	query.SetPredicate(ast.NewAndExprNode(query.GetPredicate(), notFabricOnly.GetPredicate()))
	return nil
}

func (self *EdgeServiceManager) ReadForIdentity(id string, identityId string, configTypes map[string]struct{}, isMgmtAccess bool) (*ServiceDetail, error) {
	var service *ServiceDetail
	err := self.GetDb().View(func(tx *bbolt.Tx) error {
		var err error
		service, err = self.ReadForIdentityInTx(tx, id, identityId, configTypes, isMgmtAccess)
		return err
	})
	return service, err
}

func (self *EdgeServiceManager) IsDialableByIdentity(id string, identityId string) (bool, error) {
	result := false
	err := self.GetDb().View(func(tx *bbolt.Tx) error {
		result = self.env.GetStores().Service.IsDialableByIdentity(tx, id, identityId)
		return nil
	})
	return result, err
}

func (self *EdgeServiceManager) IsBindableByIdentity(id string, identityId string) (bool, error) {
	result := false
	err := self.GetDb().View(func(tx *bbolt.Tx) error {
		result = self.env.GetStores().Service.IsBindableByIdentity(tx, id, identityId)
		return nil
	})
	return result, err
}

func (self *EdgeServiceManager) ReadForIdentityInTx(tx *bbolt.Tx, id string, identityId string, configTypes map[string]struct{}, isMgmtAccess bool) (*ServiceDetail, error) {
	edgeServiceStore := self.env.GetStores().Service
	isBindable := edgeServiceStore.IsBindableByIdentity(tx, id, identityId)
	isDialable := edgeServiceStore.IsDialableByIdentity(tx, id, identityId)

	if !isBindable && !isDialable && !isMgmtAccess { // admin can view services even if policies don't permit bind/dial {
		return nil, boltz.NewNotFoundError(self.GetStore().GetSingularEntityType(), "id", id)
	}

	// readInTx returns NotFound for fabric-only services, so this guards every caller path
	// (mgmt and identity) regardless of bind/dial denorm state.
	result, err := self.readInTx(tx, id)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, boltz.NewNotFoundError(self.GetStore().GetSingularEntityType(), "id", id)
	}
	if isBindable {
		result.Permissions = append(result.Permissions, db.PolicyTypeBindName)
	}
	if isDialable {
		result.Permissions = append(result.Permissions, db.PolicyTypeDialName)
	}
	if result.Permissions == nil {
		// don't return results with no permissions, since some SDKs assume non-nil permissions
		result.Permissions = []string{db.PolicyTypeInvalidName}
	}

	if len(configTypes) > 0 {
		identityServiceConfigs := self.env.GetStores().Identity.LoadServiceConfigsByServiceAndType(tx, identityId, configTypes)
		self.mergeConfigs(tx, configTypes, result, identityServiceConfigs)
	}

	return result, err
}

func (self *EdgeServiceManager) PublicQueryForIdentity(sessionIdentity *Identity, configTypes map[string]struct{}, query ast.Query) (*ServiceListResult, error) {
	return self.QueryForIdentity(sessionIdentity.Id, configTypes, query)
}

func (self *EdgeServiceManager) PublicQueryForMgmtAccess(sessionIdentity *Identity, configTypes map[string]struct{}, query ast.Query) (*ServiceListResult, error) {
	return self.queryServices(query, sessionIdentity.Id, configTypes, true)
}

func (self *EdgeServiceManager) QueryForIdentity(identityId string, configTypes map[string]struct{}, query ast.Query) (*ServiceListResult, error) {
	return self.queryServices(query, identityId, configTypes, false)
}

func (self *EdgeServiceManager) queryServices(query ast.Query, identityId string, configTypes map[string]struct{}, isAdmin bool) (*ServiceListResult, error) {
	result := &ServiceListResult{
		manager:     self,
		identityId:  identityId,
		configTypes: configTypes,
		isAdmin:     isAdmin,
	}
	if isAdmin {
		// Exclude fabric-only services from edge service queries
		if err := self.andNotFabricOnly(query); err != nil {
			return nil, err
		}
		if err := self.PreparedListWithHandler(query, result.collect); err != nil {
			return nil, err
		}
	} else {
		cursorProvider := self.env.GetStores().Identity.GetIdentityServicesCursorProvider(identityId)
		if err := self.PreparedListIndexed(cursorProvider, query, result.collect); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (self *EdgeServiceManager) QueryRoleAttributes(queryString string) ([]string, *models.QueryMetaData, error) {
	index := self.env.GetStores().Service.GetRoleAttributesIndex()
	return self.queryRoleAttributes(index, queryString)
}

func (self *EdgeServiceManager) Marshall(entity *EdgeService) ([]byte, error) {
	tags, err := edge_cmd_pb.EncodeTags(entity.Tags)
	if err != nil {
		return nil, err
	}

	msg := &edge_cmd_pb.Service{
		Id:                 entity.Id,
		Name:               entity.Name,
		MaxIdleTime:        int64(entity.MaxIdleTime),
		Tags:               tags,
		TerminatorStrategy: entity.TerminatorStrategy,
		RoleAttributes:     entity.RoleAttributes,
		Configs:            entity.Configs,
		EncryptionRequired: entity.EncryptionRequired,
	}

	return proto.Marshal(msg)
}

func (self *EdgeServiceManager) Unmarshall(bytes []byte) (*EdgeService, error) {
	msg := &edge_cmd_pb.Service{}
	if err := proto.Unmarshal(bytes, msg); err != nil {
		return nil, err
	}

	return &EdgeService{
		BaseEntity: models.BaseEntity{
			Id:   msg.Id,
			Tags: edge_cmd_pb.DecodeTags(msg.Tags),
		},
		Name:               msg.Name,
		MaxIdleTime:        time.Duration(msg.MaxIdleTime),
		TerminatorStrategy: msg.TerminatorStrategy,
		RoleAttributes:     msg.RoleAttributes,
		Configs:            msg.Configs,
		EncryptionRequired: msg.EncryptionRequired,
	}, nil
}

type ServiceListResult struct {
	manager     *EdgeServiceManager
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

	identityServiceConfigs := result.manager.env.GetStores().Identity.LoadServiceConfigsByServiceAndType(tx, result.identityId, result.configTypes)

	for _, key := range ids {
		// service permissions for admin & non-admin identities will be set according to policies
		service, err = result.manager.ReadForIdentityInTx(tx, key, result.identityId, result.configTypes, result.isAdmin)
		if err != nil {
			return err
		}
		result.manager.mergeConfigs(tx, result.configTypes, service, identityServiceConfigs)
		result.Services = append(result.Services, service)
	}
	return nil
}

func (self *EdgeServiceManager) mergeConfigs(tx *bbolt.Tx, configTypes map[string]struct{}, service *ServiceDetail,
	identityServiceConfigs map[string]map[string]map[string]interface{}) {
	service.Config = map[string]map[string]interface{}{}

	_, wantsAll := configTypes["all"]

	configTypeStore := self.env.GetStores().ConfigType

	if len(configTypes) > 0 && len(service.Configs) > 0 {
		configStore := self.env.GetStores().Config
		for _, configId := range service.Configs {
			config, _ := configStore.LoadById(tx, configId)
			if config != nil {
				_, wantsConfig := configTypes[config.TypeId]
				if wantsAll || wantsConfig {
					service.Config[config.TypeId] = config.Data
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

type PolicyPostureChecks struct {
	PostureChecks []*PostureCheck
	PolicyType    db.PolicyType
	PolicyName    string
}

func (self *EdgeServiceManager) GetPolicyPostureChecks(identityId, serviceId string) map[string]*PolicyPostureChecks {
	policyIdToChecks := map[string]*PolicyPostureChecks{}

	postureCheckCache := map[string]*PostureCheck{}

	servicePolicyStore := self.env.GetStores().ServicePolicy
	postureCheckLinks := servicePolicyStore.GetLinkCollection(db.EntityTypePostureChecks)
	serviceLinks := servicePolicyStore.GetLinkCollection(db.EntityTypeServices)

	policyNameSymbol := self.env.GetStores().ServicePolicy.GetSymbol(db.FieldName)
	policyTypeSymbol := self.env.GetStores().ServicePolicy.GetSymbol(db.FieldServicePolicyType)

	_ = self.GetDb().View(func(tx *bbolt.Tx) error {
		if !self.env.GetStores().PostureCheck.IterateIds(tx, ast.BoolNodeTrue).IsValid() {
			return nil
		}

		policyCursor := self.env.GetStores().Identity.GetRelatedEntitiesCursor(tx, identityId, db.EntityTypeServicePolicies, true)
		policyCursor = ast.NewFilteredCursor(policyCursor, func(policyId []byte) bool {
			return serviceLinks.IsLinked(tx, policyId, []byte(serviceId))
		})

		for policyCursor.IsValid() {
			policyIdBytes := policyCursor.Current()
			policyIdStr := string(policyIdBytes)
			policyCursor.Next()

			policyName := boltz.FieldToString(policyNameSymbol.Eval(tx, policyIdBytes))
			policyType := db.PolicyTypeDial
			if fieldType, policyTypeValue := policyTypeSymbol.Eval(tx, policyIdBytes); fieldType == boltz.TypeString {
				policyType = db.PolicyType(policyTypeValue)
			}

			//required to provide an entry for policies w/ no checks
			policyIdToChecks[policyIdStr] = &PolicyPostureChecks{
				PostureChecks: []*PostureCheck{},
				PolicyType:    policyType,
				PolicyName:    *policyName,
			}

			cursor := postureCheckLinks.IterateLinks(tx, policyIdBytes)
			for cursor.IsValid() {
				checkId := string(cursor.Current())
				if postureCheck, found := postureCheckCache[checkId]; !found {
					postureCheck, _ := self.env.GetManagers().PostureCheck.readInTx(tx, checkId)
					postureCheckCache[checkId] = postureCheck
					policyIdToChecks[policyIdStr].PostureChecks = append(policyIdToChecks[policyIdStr].PostureChecks, postureCheck)
				} else {
					policyIdToChecks[policyIdStr].PostureChecks = append(policyIdToChecks[policyIdStr].PostureChecks, postureCheck)
				}
				cursor.Next()
			}
		}
		return nil
	})

	return policyIdToChecks
}

type ServiceDetailLister struct {
	manager *EdgeServiceManager
}

func (self *ServiceDetailLister) GetListStore() boltz.Store {
	return self.manager.GetListStore()
}

func (self *ServiceDetailLister) BaseLoadInTx(tx *bbolt.Tx, id string) (*ServiceDetail, error) {
	return self.manager.readInTx(tx, id)
}

func (self *ServiceDetailLister) BasePreparedList(query ast.Query) (*models.EntityListResult[*ServiceDetail], error) {
	result := &models.EntityListResult[*ServiceDetail]{
		Loader: self,
	}

	if err := self.manager.andNotFabricOnly(query); err != nil {
		return nil, err
	}

	if err := self.manager.PreparedListWithHandler(query, result.Collect); err != nil {
		return nil, err
	}

	return result, nil
}

func (self *ServiceDetailLister) BasePreparedListIndexed(cursorProvider ast.SetCursorProvider, query ast.Query) (*models.EntityListResult[*ServiceDetail], error) {
	result := &models.EntityListResult[*ServiceDetail]{
		Loader: self,
	}

	if err := self.manager.andNotFabricOnly(query); err != nil {
		return nil, err
	}

	if err := self.manager.PreparedListIndexed(cursorProvider, query, result.Collect); err != nil {
		return nil, err
	}

	return result, nil
}
