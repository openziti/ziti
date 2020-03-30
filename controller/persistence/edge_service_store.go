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

package persistence

import (
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-fabric/controller/db"
	"github.com/netfoundry/ziti-foundation/storage/ast"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/netfoundry/ziti-foundation/util/errorz"
	"go.etcd.io/bbolt"
	"reflect"
)

type EdgeService struct {
	db.Service
	Name           string
	RoleAttributes []string
	Configs        []string
}

const (
	FieldServiceClusters = "clusters"
)

func newEdgeService(name string, roleAttributes ...string) *EdgeService {
	return &EdgeService{
		Service: db.Service{
			BaseExtEntity: boltz.BaseExtEntity{Id: uuid.New().String()},
		},
		Name:           name,
		RoleAttributes: roleAttributes,
	}
}

func (entity *EdgeService) LoadValues(store boltz.CrudStore, bucket *boltz.TypedBucket) {
	_, err := store.GetParentStore().BaseLoadOneById(bucket.Tx(), entity.Id, &entity.Service)
	bucket.SetError(err)

	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
	entity.RoleAttributes = bucket.GetStringList(FieldRoleAttributes)
	entity.Configs = bucket.GetStringList(EntityTypeConfigs)
}

func (entity *EdgeService) SetValues(ctx *boltz.PersistContext) {
	entity.Service.SetValues(ctx.GetParentContext())

	entity.SetBaseValues(ctx)
	store := ctx.Store.(*edgeServiceStoreImpl)
	ctx.SetString(FieldName, entity.Name)
	ctx.SetString(FieldName, entity.Name)
	ctx.SetStringList(FieldRoleAttributes, entity.RoleAttributes)
	ctx.SetLinkedIds(EntityTypeConfigs, entity.Configs)

	// index change won't fire if we don't have any roles on create, but we need to evaluate if we match any #all roles
	if ctx.IsCreate && len(entity.RoleAttributes) == 0 {
		store.rolesChanged(ctx.Bucket.Tx(), []byte(entity.Id), nil, nil, ctx.Bucket)
	}
}

func (entity *EdgeService) GetEntityType() string {
	return EntityTypeServices
}

func (entity *EdgeService) GetName() string {
	return entity.Name
}

type EdgeServiceStore interface {
	NameIndexedStore

	LoadOneById(tx *bbolt.Tx, id string) (*EdgeService, error)
	LoadOneByName(tx *bbolt.Tx, id string) (*EdgeService, error)

	GetRoleAttributesIndex() boltz.SetReadIndex
	GetRoleAttributesCursorProvider(values []string, semantic string) (ast.SetCursorProvider, error)
}

func newEdgeServiceStore(stores *stores) *edgeServiceStoreImpl {
	store := &edgeServiceStoreImpl{
		baseStore: newChildBaseStore(stores, stores.Service, EntityTypeServices),
	}
	store.InitImpl(store)
	return store
}

type edgeServiceStoreImpl struct {
	*baseStore

	indexName           boltz.ReadIndex
	indexRoleAttributes boltz.SetReadIndex
	symbolClusters      boltz.EntitySetSymbol
	symbolSessions      boltz.EntitySetSymbol

	symbolServicePolicies           boltz.EntitySetSymbol
	symbolServiceEdgeRouterPolicies boltz.EntitySetSymbol
	symbolConfigs                   boltz.EntitySetSymbol
}

func (store *edgeServiceStoreImpl) NewStoreEntity() boltz.Entity {
	return &EdgeService{}
}

func (store *edgeServiceStoreImpl) GetRoleAttributesIndex() boltz.SetReadIndex {
	return store.indexRoleAttributes
}

func (store *edgeServiceStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()
	store.GetParentStore().GrantSymbols(store)

	store.indexName = store.addUniqueNameField()
	store.indexRoleAttributes = store.addRoleAttributesField()

	store.symbolClusters = store.AddFkSetSymbol(FieldServiceClusters, store.stores.cluster)
	store.symbolSessions = store.AddFkSetSymbol(EntityTypeSessions, store.stores.session)
	store.symbolServiceEdgeRouterPolicies = store.AddFkSetSymbol(EntityTypeServiceEdgeRouterPolicies, store.stores.serviceEdgeRouterPolicy)
	store.symbolServicePolicies = store.AddFkSetSymbol(EntityTypeServicePolicies, store.stores.servicePolicy)
	store.symbolConfigs = store.AddFkSetSymbol(EntityTypeConfigs, store.stores.config)

	store.indexRoleAttributes.AddListener(store.rolesChanged)
}

func (store *edgeServiceStoreImpl) rolesChanged(tx *bbolt.Tx, rowId []byte, _ []boltz.FieldTypeAndValue, new []boltz.FieldTypeAndValue, holder errorz.ErrorHolder) {
	// Recalculate service policy links
	rolesSymbol := store.stores.servicePolicy.symbolServiceRoles
	linkCollection := store.stores.servicePolicy.serviceCollection
	semanticSymbol := store.stores.servicePolicy.symbolSemantic
	UpdateRelatedRoles(tx, string(rowId), rolesSymbol, linkCollection, new, holder, semanticSymbol)

	// Recalculate service edge router policy links
	rolesSymbol = store.stores.serviceEdgeRouterPolicy.symbolServiceRoles
	linkCollection = store.stores.serviceEdgeRouterPolicy.serviceCollection
	semanticSymbol = store.stores.serviceEdgeRouterPolicy.symbolSemantic
	UpdateRelatedRoles(tx, string(rowId), rolesSymbol, linkCollection, new, holder, semanticSymbol)
}

func (store *edgeServiceStoreImpl) initializeLinked() {
	store.AddLinkCollection(store.symbolClusters, store.stores.cluster.symbolServices)
	store.AddLinkCollection(store.symbolServiceEdgeRouterPolicies, store.stores.serviceEdgeRouterPolicy.symbolServices)
	store.AddLinkCollection(store.symbolServicePolicies, store.stores.servicePolicy.symbolServices)
	store.AddLinkCollection(store.symbolConfigs, store.stores.config.symbolServices)

	store.EventEmmiter.AddListener(boltz.EventUpdate, func(i ...interface{}) {
		if len(i) != 1 {
			return
		}
		service, ok := i[0].(*EdgeService)
		if !ok {
			pfxlog.Logger().Warnf("unexpected type in edge service event: %v", reflect.TypeOf(i[0]))
			return
		}
		store.stores.DbProvider.GetServiceCache().RemoveFromCache(service.Id)
		pfxlog.Logger().WithField("id", service).Debugf("removed service from fabric cache")
	})
}

func (store *edgeServiceStoreImpl) GetNameIndex() boltz.ReadIndex {
	return store.indexName
}

func (store *edgeServiceStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*EdgeService, error) {
	entity := &EdgeService{}
	if err := store.baseLoadOneById(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *edgeServiceStoreImpl) LoadOneByName(tx *bbolt.Tx, name string) (*EdgeService, error) {
	id := store.indexName.Read(tx, []byte(name))
	if id != nil {
		return store.LoadOneById(tx, string(id))
	}
	return nil, nil
}

func (store *edgeServiceStoreImpl) LoadOneByQuery(tx *bbolt.Tx, query string) (*EdgeService, error) {
	entity := &EdgeService{}
	if found, err := store.BaseLoadOneByQuery(tx, query, entity); !found || err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *edgeServiceStoreImpl) DeleteById(ctx boltz.MutateContext, id string) error {
	for _, sessionId := range store.GetRelatedEntitiesIdList(ctx.Tx(), id, EntityTypeSessions) {
		if err := store.stores.session.DeleteById(ctx, sessionId); err != nil {
			return err
		}
	}

	if entity, _ := store.LoadOneById(ctx.Tx(), id); entity != nil {
		// Remove entity from ServiceRoles in service policies
		if err := store.deleteEntityReferences(ctx.Tx(), entity, store.stores.servicePolicy.symbolServiceRoles); err != nil {
			return err
		}

		// Remove entity from ServiceRoles in service edge router policies
		if err := store.deleteEntityReferences(ctx.Tx(), entity, store.stores.serviceEdgeRouterPolicy.symbolServiceRoles); err != nil {
			return err
		}
	}

	return store.BaseStore.DeleteById(ctx, id)
}

func (store *edgeServiceStoreImpl) GetRoleAttributesCursorProvider(values []string, semantic string) (ast.SetCursorProvider, error) {
	return store.getRoleAttributesCursorProvider(store.indexRoleAttributes, values, semantic)
}
