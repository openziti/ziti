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

package db

import (
	"fmt"
	"time"

	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/storage/ast"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/openziti/ziti/v2/controller/xt"
	"github.com/openziti/ziti/v2/controller/xt_smartrouting"
	"go.etcd.io/bbolt"
)

const (
	EntityTypeServices             = "services"
	FieldServiceTerminatorStrategy = "terminatorStrategy"
	FieldServiceMaxIdleTime        = "maxIdleTime"
	FieldServiceEncryptionRequired = "encryptionRequired"
	FieldServiceIsFabricOnly       = "isFabricOnly"

	FieldEdgeServiceDialIdentities = "dialIdentities"
	FieldEdgeServiceBindIdentities = "bindIdentities"
	FieldServiceIdentityService    = "identityServices"
)

type Service struct {
	boltz.BaseExtEntity
	Name               string        `json:"name"`
	MaxIdleTime        time.Duration `json:"maxIdleTime"`
	TerminatorStrategy string        `json:"terminatorStrategy"`
	RoleAttributes     []string      `json:"roleAttributes"`
	Configs            []string      `json:"configs"`
	EncryptionRequired bool          `json:"encryptionRequired"`
	IsFabricOnly       bool          `json:"isFabricOnly"`
}

func (entity *Service) GetEntityType() string {
	return EntityTypeServices
}

func (entity *Service) GetName() string {
	return entity.Name
}

type ServiceStore interface {
	Store[*Service]
	boltz.EntityStrategy[*Service]
	GetNameIndex() boltz.ReadIndex
	FindByName(tx *bbolt.Tx, name string) (*Service, error)
	IsBindableByIdentity(tx *bbolt.Tx, id string, identityId string) bool
	IsDialableByIdentity(tx *bbolt.Tx, id string, identityId string) bool
	GetRoleAttributesIndex() boltz.SetReadIndex
	GetRoleAttributesCursorProvider(values []string, semantic string) (ast.SetCursorProvider, error)
}

func newServiceStore(stores *stores) *serviceStoreImpl {
	store := &serviceStoreImpl{}
	store.baseStore = baseStore[*Service]{
		stores:    stores,
		BaseStore: boltz.NewBaseStore(NewStoreDefinition[*Service](store)),
	}
	store.InitImpl(store)
	return store
}

func newEdgeService(name string, roleAttributes ...string) *Service {
	return &Service{
		BaseExtEntity:  boltz.BaseExtEntity{Id: eid.New()},
		Name:           name,
		RoleAttributes: roleAttributes,
	}
}

type serviceStoreImpl struct {
	baseStore[*Service]

	indexName           boltz.ReadIndex
	indexRoleAttributes boltz.SetReadIndex

	terminatorsSymbol boltz.EntitySetSymbol

	symbolRoleAttributes boltz.EntitySetSymbol
	symbolConfigs        boltz.EntitySetSymbol

	symbolServicePolicies           boltz.EntitySetSymbol
	symbolServiceEdgeRouterPolicies boltz.EntitySetSymbol

	symbolDialIdentities boltz.EntitySetSymbol
	symbolBindIdentities boltz.EntitySetSymbol
	symbolEdgeRouters    boltz.EntitySetSymbol

	bindIdentitiesCollection boltz.RefCountedLinkCollection
	dialIdentitiesCollection boltz.RefCountedLinkCollection
	edgeRoutersCollection    boltz.RefCountedLinkCollection

	symbolIdentityServices boltz.EntitySetSymbol
	identityServicesLinks  *boltz.LinkedSetSymbol
}

func (store *serviceStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()

	// Fabric fields
	symbolName := store.AddSymbol(FieldName, ast.NodeTypeString)
	store.indexName = store.AddUniqueIndex(symbolName)
	store.AddSymbol(FieldServiceTerminatorStrategy, ast.NodeTypeString)
	store.terminatorsSymbol = store.AddFkSetSymbol(EntityTypeTerminators, store.stores.terminator)

	// Edge fields
	store.symbolRoleAttributes = store.AddPublicSetSymbol(FieldRoleAttributes, ast.NodeTypeString)
	store.indexRoleAttributes = store.AddSetIndex(store.symbolRoleAttributes)

	store.AddSymbol(FieldServiceIsFabricOnly, ast.NodeTypeBool)

	store.symbolServiceEdgeRouterPolicies = store.AddFkSetSymbol(EntityTypeServiceEdgeRouterPolicies, store.stores.serviceEdgeRouterPolicy)
	store.symbolServicePolicies = store.AddFkSetSymbol(EntityTypeServicePolicies, store.stores.servicePolicy)
	store.symbolConfigs = store.AddFkSetSymbol(EntityTypeConfigs, store.stores.config)
	store.MakeSymbolPublic(EntityTypeConfigs)

	store.symbolBindIdentities = store.AddFkSetSymbol(FieldEdgeServiceBindIdentities, store.stores.identity)
	store.symbolDialIdentities = store.AddFkSetSymbol(FieldEdgeServiceDialIdentities, store.stores.identity)

	store.symbolEdgeRouters = store.AddFkSetSymbol(FieldEdgeRouters, store.stores.edgeRouter)

	store.indexRoleAttributes.AddListener(store.rolesChanged)

	store.symbolIdentityServices = store.AddSetSymbol(FieldServiceIdentityService, ast.NodeTypeOther)
	store.identityServicesLinks = &boltz.LinkedSetSymbol{EntitySymbol: store.symbolIdentityServices}
}

func (store *serviceStoreImpl) initializeLinked() {
	store.AddLinkCollection(store.symbolServiceEdgeRouterPolicies, store.stores.serviceEdgeRouterPolicy.symbolServices)
	store.AddLinkCollection(store.symbolServicePolicies, store.stores.servicePolicy.symbolServices)
	store.AddLinkCollection(store.symbolConfigs, store.stores.config.symbolServices)

	store.bindIdentitiesCollection = store.AddRefCountedLinkCollection(store.symbolBindIdentities, store.stores.identity.symbolBindServices)
	store.dialIdentitiesCollection = store.AddRefCountedLinkCollection(store.symbolDialIdentities, store.stores.identity.symbolDialServices)
	store.edgeRoutersCollection = store.AddRefCountedLinkCollection(store.symbolEdgeRouters, store.stores.edgeRouter.symbolServices)
}

func (store *serviceStoreImpl) GetNameIndex() boltz.ReadIndex {
	return store.indexName
}

func (store *serviceStoreImpl) GetRoleAttributesIndex() boltz.SetReadIndex {
	return store.indexRoleAttributes
}

func (store *serviceStoreImpl) NewEntity() *Service {
	return &Service{}
}

func (store *serviceStoreImpl) FillEntity(entity *Service, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
	entity.TerminatorStrategy = bucket.GetStringWithDefault(FieldServiceTerminatorStrategy, "")
	entity.MaxIdleTime = time.Duration(bucket.GetInt64WithDefault(FieldServiceMaxIdleTime, 0))

	// Edge fields
	entity.RoleAttributes = bucket.GetStringList(FieldRoleAttributes)
	entity.Configs = bucket.GetStringList(EntityTypeConfigs)
	entity.EncryptionRequired = bucket.GetBoolWithDefault(FieldServiceEncryptionRequired, true)
	entity.IsFabricOnly = bucket.GetBoolWithDefault(FieldServiceIsFabricOnly, false)
}

func (store *serviceStoreImpl) PersistEntity(entity *Service, ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldName, entity.Name)
	ctx.SetInt64(FieldServiceMaxIdleTime, int64(entity.MaxIdleTime))

	if entity.TerminatorStrategy == "" {
		entity.TerminatorStrategy = xt_smartrouting.Name
	}
	_, changed := ctx.GetAndSetString(FieldServiceTerminatorStrategy, entity.TerminatorStrategy)
	if changed {
		strategy, err := xt.GlobalRegistry().GetStrategy(entity.TerminatorStrategy)
		if err != nil {
			ctx.Bucket.SetError(err)
			return
		}

		if !ctx.IsCreate {
			terminators, err := store.getTerminators(ctx.Bucket.Tx(), entity.Id)
			if !ctx.Bucket.SetError(err) {
				event := xt.NewStrategyChangeEvent(entity.Id, nil, terminators, nil, nil)
				ctx.Bucket.SetError(strategy.HandleTerminatorChange(event))
			}
		}
	}

	// Edge fields
	store.validateRoleAttributes(entity.RoleAttributes, ctx.Bucket)
	ctx.SetStringList(FieldRoleAttributes, entity.RoleAttributes)
	ctx.SetLinkedIds(EntityTypeConfigs, entity.Configs)
	ctx.SetBool(FieldServiceEncryptionRequired, entity.EncryptionRequired)
	ctx.SetBool(FieldServiceIsFabricOnly, entity.IsFabricOnly)

	// index change won't fire if we don't have any roles on create, but we need to evaluate if we match any #all roles.
	// Fabric-only services should never be evaluated against edge policies.
	if ctx.IsCreate && !entity.IsFabricOnly && len(entity.RoleAttributes) == 0 {
		store.rolesChanged(ctx.MutateContext, []byte(entity.Id), nil, nil, ctx.Bucket)
	}
}

func (store *serviceStoreImpl) FindByName(tx *bbolt.Tx, name string) (*Service, error) {
	id := store.indexName.Read(tx, []byte(name))
	if id != nil {
		entity, _, err := store.FindById(tx, string(id))
		return entity, err
	}
	return nil, nil
}

func (store *serviceStoreImpl) DeleteById(ctx boltz.MutateContext, id string) error {
	// Clean up edge service references before deleting
	if err := store.cleanupEdgeService(ctx, id); err != nil {
		return err
	}

	// Clean up terminators
	terminatorIds := store.GetRelatedEntitiesIdList(ctx.Tx(), id, EntityTypeTerminators)
	for _, terminatorId := range terminatorIds {
		if err := store.stores.terminator.DeleteById(ctx, terminatorId); err != nil {
			return err
		}
	}
	return store.BaseStore.DeleteById(ctx, id)
}

func (store *serviceStoreImpl) Update(ctx boltz.MutateContext, entity *Service, checker boltz.FieldChecker) error {
	if result := store.BaseStore.Update(ctx, entity, checker); result != nil {
		return result
	}

	// Dispatch service update events if this is an edge service
	if !entity.IsFabricOnly {
		id := entity.GetId()
		var servicePolicyEvents []*ServiceEvent

		cursor := store.dialIdentitiesCollection.IterateLinks(ctx.Tx(), []byte(id), true)
		for cursor.IsValid() {
			servicePolicyEvents = append(servicePolicyEvents, &ServiceEvent{
				Type:       ServiceUpdated,
				IdentityId: string(cursor.Current()),
				ServiceId:  id,
			})
			cursor.Next()
		}

		cursor = store.bindIdentitiesCollection.IterateLinks(ctx.Tx(), []byte(id), true)
		for cursor.IsValid() {
			servicePolicyEvents = append(servicePolicyEvents, &ServiceEvent{
				Type:       ServiceUpdated,
				IdentityId: string(cursor.Current()),
				ServiceId:  id,
			})
			cursor.Next()
		}

		if len(servicePolicyEvents) > 0 {
			ctx.Tx().OnCommit(func() {
				ServiceEvents.dispatchEventsAsync(servicePolicyEvents)
			})
		}
	}

	return nil
}

func (store *serviceStoreImpl) cleanupEdgeService(ctx boltz.MutateContext, id string) error {
	if entity, _ := store.LoadById(ctx.Tx(), id); entity != nil {
		// Skip cleanup for fabric-only services (they have no edge references)
		if entity.IsFabricOnly {
			return nil
		}

		// Remove entity from ServiceRoles in service policies
		if err := store.deleteEntityReferences(ctx.Tx(), entity, store.stores.servicePolicy.symbolServiceRoles); err != nil {
			return err
		}

		// Remove entity from ServiceRoles in service edge router policies
		if err := store.deleteEntityReferences(ctx.Tx(), entity, store.stores.serviceEdgeRouterPolicy.symbolServiceRoles); err != nil {
			return err
		}

		if len(entity.RoleAttributes) != 0 {
			entity.RoleAttributes = nil
			if err := store.Update(ctx, entity, nil); err != nil {
				return fmt.Errorf("could not clear role attributes for service '%s' before deletion (%w)", id, err)
			}
		}

		var servicePolicyEvents []*ServiceEvent

		cursor := store.dialIdentitiesCollection.IterateLinks(ctx.Tx(), []byte(id), true)
		for cursor.IsValid() {
			servicePolicyEvents = append(servicePolicyEvents, &ServiceEvent{
				Type:       ServiceDialAccessLost,
				IdentityId: string(cursor.Current()),
				ServiceId:  id,
			})
			cursor.Next()
		}

		cursor = store.bindIdentitiesCollection.IterateLinks(ctx.Tx(), []byte(id), true)
		for cursor.IsValid() {
			servicePolicyEvents = append(servicePolicyEvents, &ServiceEvent{
				Type:       ServiceBindAccessLost,
				IdentityId: string(cursor.Current()),
				ServiceId:  id,
			})
			cursor.Next()
		}

		if len(servicePolicyEvents) > 0 {
			ctx.Tx().OnCommit(func() {
				ServiceEvents.dispatchEventsAsync(servicePolicyEvents)
			})
		}

		err := store.symbolIdentityServices.Map(ctx.Tx(), []byte(id), func(mapCtx *boltz.MapContext) {
			identityId := mapCtx.ValueS()
			err := store.stores.identity.removeServiceConfigs(ctx.Tx(), identityId, false, func(serviceId, _, _ string) bool {
				return serviceId == id
			})
			mapCtx.SetError(err)
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func (store *serviceStoreImpl) getTerminators(tx *bbolt.Tx, serviceId string) ([]xt.Terminator, error) {
	var terminators []xt.Terminator
	for _, tId := range store.GetRelatedEntitiesIdList(tx, serviceId, EntityTypeTerminators) {
		terminator, _, err := store.stores.terminator.FindById(tx, tId)
		if err != nil {
			return nil, err
		}
		if terminator != nil {
			terminators = append(terminators, terminator)
		}
	}
	return terminators, nil
}

// rolesChanged recalculates service-policy and service-edge-router-policy links for a service whose
// role attributes changed. It does not itself filter fabric-only services, and does not need to:
// fabric-only services never carry role attributes. That invariant is enforced upstream - the fabric
// model.Service type has no RoleAttributes field, the edge manager only ever writes IsFabricOnly=false
// services, and the v45 migration preserves the partition - so the role-attribute index listener
// never fires for a fabric-only row, and the only explicit call below (in PersistEntity) is guarded
// by !IsFabricOnly. If a future path can set role attributes on a fabric-only service, add a
// fabric-only short-circuit here (and filter QueryRoleAttributes) to keep them out of edge denorm.
func (store *serviceStoreImpl) rolesChanged(mutateCtx boltz.MutateContext, rowId []byte, _ []boltz.FieldTypeAndValue, new []boltz.FieldTypeAndValue, holder errorz.ErrorHolder) {
	// Recalculate service policy links
	ctx := &roleAttributeChangeContext{
		mutateCtx:             mutateCtx,
		rolesSymbol:           store.stores.servicePolicy.symbolServiceRoles,
		linkCollection:        store.stores.servicePolicy.serviceCollection,
		relatedLinkCollection: store.stores.servicePolicy.identityCollection,
		ErrorHolder:           holder,
	}
	store.updateServicePolicyRelatedRoles(ctx, rowId, new)

	// Recalculate service edge router policy links
	ctx = &roleAttributeChangeContext{
		mutateCtx:             mutateCtx,
		rolesSymbol:           store.stores.serviceEdgeRouterPolicy.symbolServiceRoles,
		linkCollection:        store.stores.serviceEdgeRouterPolicy.serviceCollection,
		relatedLinkCollection: store.stores.serviceEdgeRouterPolicy.edgeRouterCollection,
		denormLinkCollection:  store.edgeRoutersCollection,
		ErrorHolder:           holder,
	}
	UpdateRelatedRoles(ctx, rowId, new, store.stores.serviceEdgeRouterPolicy.symbolSemantic)
}

// isNotFabricOnly reports whether the given service is an edge service (not fabric-only). It backs
// the store-level edge-API guards (identity service-config overrides, policy @id validation).
//
// TEMPORARY(fabric-edge-collapse): remove with the fabric/edge split; see EdgeServiceManager's
// notFoundIfFabricOnly for the full picture.
func (store *serviceStoreImpl) isNotFabricOnly(tx *bbolt.Tx, entityId []byte) bool {
	isFabricOnly := boltz.FieldToBool(store.GetSymbol(FieldServiceIsFabricOnly).Eval(tx, entityId))
	return isFabricOnly == nil || !*isFabricOnly
}

func (store *serviceStoreImpl) GetRoleAttributesCursorProvider(values []string, semantic string) (ast.SetCursorProvider, error) {
	return store.getRoleAttributesCursorProvider(store.indexRoleAttributes, values, semantic)
}

func (store *serviceStoreImpl) IsBindableByIdentity(tx *bbolt.Tx, id string, identityId string) bool {
	linkCount := store.bindIdentitiesCollection.GetLinkCount(tx, []byte(id), []byte(identityId))
	return linkCount != nil && *linkCount > 0
}

func (store *serviceStoreImpl) IsDialableByIdentity(tx *bbolt.Tx, id string, identityId string) bool {
	linkCount := store.dialIdentitiesCollection.GetLinkCount(tx, []byte(id), []byte(identityId))
	return linkCount != nil && *linkCount > 0
}
