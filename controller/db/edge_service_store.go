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
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/common/eid"
	"go.etcd.io/bbolt"
)

const (
	FieldEdgeServiceDialIdentities = "dialIdentities"
	FieldEdgeServiceBindIdentities = "bindIdentities"
	FieldServiceEncryptionRequired = "encryptionRequired"
)

type EdgeService struct {
	Service
	RoleAttributes     []string `json:"roleAttributes"`
	Configs            []string `json:"configs"`
	EncryptionRequired bool     `json:"encryptionRequired"`
}

func newEdgeService(name string, roleAttributes ...string) *EdgeService {
	return &EdgeService{
		Service: Service{
			BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
			Name:          name,
		},
		RoleAttributes: roleAttributes,
	}
}

var _ EdgeServiceStore = (*edgeServiceStoreImpl)(nil)

type EdgeServiceStore interface {
	NameIndexed
	Store[*EdgeService]

	IsBindableByIdentity(tx *bbolt.Tx, id string, identityId string) bool
	IsDialableByIdentity(tx *bbolt.Tx, id string, identityId string) bool
	GetRoleAttributesIndex() boltz.SetReadIndex
	GetRoleAttributesCursorProvider(values []string, semantic string) (ast.SetCursorProvider, error)
}

func newEdgeServiceStore(stores *stores) *edgeServiceStoreImpl {
	parentMapper := func(entity boltz.Entity) boltz.Entity {
		if edgeService, ok := entity.(*EdgeService); ok {
			return &edgeService.Service
		}
		return entity
	}

	store := &edgeServiceStoreImpl{}
	store.baseStore = newChildBaseStore[*EdgeService](stores, parentMapper, store, stores.service, EdgeBucket)
	store.InitImpl(store)

	stores.service.RegisterChildStoreStrategy(store)

	return store
}

type edgeServiceStoreImpl struct {
	*baseStore[*EdgeService]

	indexName           boltz.ReadIndex
	indexRoleAttributes boltz.SetReadIndex

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
}

func (store *edgeServiceStoreImpl) HandleUpdate(ctx boltz.MutateContext, entity *Service, checker boltz.FieldChecker) (bool, error) {
	edgeService, found, err := store.FindById(ctx.Tx(), entity.Id)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}

	edgeService.Service = *entity
	return true, store.Update(ctx, edgeService, checker)
}

func (store *edgeServiceStoreImpl) HandleDelete(ctx boltz.MutateContext, entity *Service) error {
	return store.cleanupEdgeService(ctx, entity.Id)
}

func (store *edgeServiceStoreImpl) GetStore() boltz.Store {
	return store
}

func (store *edgeServiceStoreImpl) GetRoleAttributesIndex() boltz.SetReadIndex {
	return store.indexRoleAttributes
}

func (store *edgeServiceStoreImpl) initializeLocal() {
	store.GetParentStore().GrantSymbols(store)

	store.symbolRoleAttributes = store.AddPublicSetSymbol(FieldRoleAttributes, ast.NodeTypeString)

	store.indexName = store.GetParentStore().(ServiceStore).GetNameIndex()
	store.indexRoleAttributes = store.AddSetIndex(store.symbolRoleAttributes)

	store.symbolServiceEdgeRouterPolicies = store.AddFkSetSymbol(EntityTypeServiceEdgeRouterPolicies, store.stores.serviceEdgeRouterPolicy)
	store.symbolServicePolicies = store.AddFkSetSymbol(EntityTypeServicePolicies, store.stores.servicePolicy)
	store.symbolConfigs = store.AddFkSetSymbol(EntityTypeConfigs, store.stores.config)
	store.MakeSymbolPublic(EntityTypeConfigs)

	store.symbolBindIdentities = store.AddFkSetSymbol(FieldEdgeServiceBindIdentities, store.stores.identity)
	store.symbolDialIdentities = store.AddFkSetSymbol(FieldEdgeServiceDialIdentities, store.stores.identity)

	// TODO: migrate this field name to routers, to match identity store
	store.symbolEdgeRouters = store.AddFkSetSymbol(FieldEdgeRouters, store.stores.edgeRouter)

	store.indexRoleAttributes.AddListener(store.rolesChanged)
}

func (store *edgeServiceStoreImpl) initializeLinked() {
	store.AddLinkCollection(store.symbolServiceEdgeRouterPolicies, store.stores.serviceEdgeRouterPolicy.symbolServices)
	store.AddLinkCollection(store.symbolServicePolicies, store.stores.servicePolicy.symbolServices)
	store.AddLinkCollection(store.symbolConfigs, store.stores.config.symbolServices)

	store.bindIdentitiesCollection = store.AddRefCountedLinkCollection(store.symbolBindIdentities, store.stores.identity.symbolBindServices)
	store.dialIdentitiesCollection = store.AddRefCountedLinkCollection(store.symbolDialIdentities, store.stores.identity.symbolDialServices)
	store.edgeRoutersCollection = store.AddRefCountedLinkCollection(store.symbolEdgeRouters, store.stores.edgeRouter.symbolServices)
}

func (self *edgeServiceStoreImpl) NewEntity() *EdgeService {
	return &EdgeService{}
}

func (store *edgeServiceStoreImpl) FillEntity(entity *EdgeService, bucket *boltz.TypedBucket) {
	store.stores.service.FillEntity(&entity.Service, store.getParentBucket(entity, bucket))

	entity.RoleAttributes = bucket.GetStringList(FieldRoleAttributes)
	entity.Configs = bucket.GetStringList(EntityTypeConfigs)

	//default to true for old services w/o any value explicitly set
	entity.EncryptionRequired = bucket.GetBoolWithDefault(FieldServiceEncryptionRequired, true)
}

func (store *edgeServiceStoreImpl) PersistEntity(entity *EdgeService, ctx *boltz.PersistContext) {
	store.stores.service.PersistEntity(&entity.Service, ctx.GetParentContext())

	ctx.SetString(FieldName, entity.Name)
	store.validateRoleAttributes(entity.RoleAttributes, ctx.Bucket)
	ctx.SetStringList(FieldRoleAttributes, entity.RoleAttributes)
	ctx.SetLinkedIds(EntityTypeConfigs, entity.Configs)
	ctx.SetBool(FieldServiceEncryptionRequired, entity.EncryptionRequired)

	// index change won't fire if we don't have any roles on create, but we need to evaluate if we match any #all roles
	if ctx.IsCreate && len(entity.RoleAttributes) == 0 {
		store.rolesChanged(ctx.MutateContext, []byte(entity.Id), nil, nil, ctx.Bucket)
	}
}

func (store *edgeServiceStoreImpl) rolesChanged(mutateCtx boltz.MutateContext, rowId []byte, _ []boltz.FieldTypeAndValue, new []boltz.FieldTypeAndValue, holder errorz.ErrorHolder) {
	// Recalculate service policy links
	ctx := &roleAttributeChangeContext{
		tx:                    mutateCtx.Tx(),
		rolesSymbol:           store.stores.servicePolicy.symbolServiceRoles,
		linkCollection:        store.stores.servicePolicy.serviceCollection,
		relatedLinkCollection: store.stores.servicePolicy.identityCollection,
		ErrorHolder:           holder,
	}
	store.updateServicePolicyRelatedRoles(ctx, rowId, new)

	// Recalculate service edge router policy links
	ctx = &roleAttributeChangeContext{
		tx:                    mutateCtx.Tx(),
		rolesSymbol:           store.stores.serviceEdgeRouterPolicy.symbolServiceRoles,
		linkCollection:        store.stores.serviceEdgeRouterPolicy.serviceCollection,
		relatedLinkCollection: store.stores.serviceEdgeRouterPolicy.edgeRouterCollection,
		denormLinkCollection:  store.edgeRoutersCollection,
		ErrorHolder:           holder,
	}
	UpdateRelatedRoles(ctx, rowId, new, store.stores.serviceEdgeRouterPolicy.symbolSemantic)
}

func (store *edgeServiceStoreImpl) GetNameIndex() boltz.ReadIndex {
	return store.indexName
}

func (store *edgeServiceStoreImpl) Update(ctx boltz.MutateContext, entity *EdgeService, checker boltz.FieldChecker) error {
	if result := store.baseStore.Update(ctx, entity, checker); result != nil {
		return result
	}

	id := entity.GetId()
	var servicePolicyEvents []*ServiceEvent

	// If a service is updated we need to notify everyone who has access to the identity, not just those who
	// have gained/lost access, since everyone will need to refresh the service. We will generate two events
	// for identities
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

	return nil
}

func (store *edgeServiceStoreImpl) cleanupEdgeService(ctx boltz.MutateContext, id string) error {
	if entity, _ := store.LoadOneById(ctx.Tx(), id); entity != nil {
		// Remove entity from ServiceRoles in service policies
		if err := store.deleteEntityReferences(ctx.Tx(), entity, store.stores.servicePolicy.symbolServiceRoles); err != nil {
			return err
		}

		// Remove entity from ServiceRoles in service edge router policies
		if err := store.deleteEntityReferences(ctx.Tx(), entity, store.stores.serviceEdgeRouterPolicy.symbolServiceRoles); err != nil {
			return err
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
	}

	return nil
}

func (store *edgeServiceStoreImpl) GetRoleAttributesCursorProvider(values []string, semantic string) (ast.SetCursorProvider, error) {
	return store.getRoleAttributesCursorProvider(store.indexRoleAttributes, values, semantic)
}

func (store *edgeServiceStoreImpl) IsBindableByIdentity(tx *bbolt.Tx, id string, identityId string) bool {
	linkCount := store.bindIdentitiesCollection.GetLinkCount(tx, []byte(id), []byte(identityId))
	return linkCount != nil && *linkCount > 0
}

func (store *edgeServiceStoreImpl) IsDialableByIdentity(tx *bbolt.Tx, id string, identityId string) bool {
	linkCount := store.dialIdentitiesCollection.GetLinkCount(tx, []byte(id), []byte(identityId))
	return linkCount != nil && *linkCount > 0
}
