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

package persistence

import (
	"reflect"
	"sort"

	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-fabric/controller/network"
	"github.com/netfoundry/ziti-foundation/storage/ast"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/netfoundry/ziti-foundation/util/errorz"
	"github.com/netfoundry/ziti-foundation/util/stringz"
	"go.etcd.io/bbolt"
)

type EdgeService struct {
	network.Service
	EdgeEntityFields
	Name            string
	DnsHostname     string
	DnsPort         uint16
	AppWans         []string
	Clusters        []string
	RoleAttributes  []string
	EdgeRouterRoles []string
	Configs         []string
}

const (
	FieldServiceDnsHostname     = "dnsHostname"
	FieldServiceDnsPort         = "dnsPort"
	FieldServiceAppwans         = "appwans"
	FieldServiceClusters        = "clusters"
	FieldServiceEdgeRouterRoles = "edgeRouterRoles"
)

func newEdgeService(name string, roleAttributes ...string) *EdgeService {
	return &EdgeService{
		Service:        network.Service{Id: uuid.New().String()},
		Name:           name,
		RoleAttributes: roleAttributes,
	}
}

func (entity *EdgeService) LoadValues(store boltz.CrudStore, bucket *boltz.TypedBucket) {
	_, err := store.GetParentStore().BaseLoadOneById(bucket.Tx(), entity.Id, &entity.Service)
	bucket.SetError(err)

	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
	entity.DnsHostname = bucket.GetStringWithDefault(FieldServiceDnsHostname, "")
	entity.DnsPort = uint16(bucket.GetInt32WithDefault(FieldServiceDnsPort, 0))
	entity.AppWans = bucket.GetStringList(FieldServiceAppwans)
	entity.Clusters = bucket.GetStringList(FieldServiceClusters)
	entity.RoleAttributes = bucket.GetStringList(FieldRoleAttributes)
	entity.EdgeRouterRoles = bucket.GetStringList(FieldServiceEdgeRouterRoles)
	entity.Configs = bucket.GetStringList(EntityTypeConfigs)
}

func (entity *EdgeService) SetValues(ctx *boltz.PersistContext) {
	entity.Service.SetValues(ctx.GetParentContext())

	entity.SetBaseValues(ctx)
	store := ctx.Store.(*edgeServiceStoreImpl)
	if ctx.IsCreate {
		ctx.SetString(FieldName, entity.Name)
	} else if oldValue, changed := ctx.GetAndSetString(FieldName, entity.Name); changed {
		store.nameChanged(ctx.Bucket, entity, *oldValue)
	}
	ctx.SetString(FieldName, entity.Name)
	ctx.SetString(FieldServiceDnsHostname, entity.DnsHostname)
	ctx.SetInt32(FieldServiceDnsPort, int32(entity.DnsPort))
	ctx.SetStringList(FieldRoleAttributes, entity.RoleAttributes)

	sort.Strings(entity.EdgeRouterRoles)
	oldRoles, valueSet := ctx.GetAndSetStringList(FieldServiceEdgeRouterRoles, entity.EdgeRouterRoles)

	if valueSet && !stringz.EqualSlices(oldRoles, entity.EdgeRouterRoles) {
		store.edgeRouterRolesChanged(ctx, entity.Id, entity.EdgeRouterRoles)
	}

	// index change won't fire if we don't have any roles on create, but we need to evaluate if we match any all roles
	if ctx.IsCreate && len(entity.RoleAttributes) == 0 {
		store.rolesChanged(ctx.Bucket.Tx(), []byte(entity.Id), nil, nil, ctx.Bucket)
	}

	ctx.SetLinkedIds(EntityTypeConfigs, entity.Configs)
}

func (entity *EdgeService) GetEntityType() string {
	return EntityTypeServices
}

func (entity *EdgeService) GetName() string {
	return entity.Name
}

type EdgeServiceStore interface {
	Store
	LoadOneById(tx *bbolt.Tx, id string) (*EdgeService, error)
	LoadOneByName(tx *bbolt.Tx, id string) (*EdgeService, error)
}

func newEdgeServiceStore(stores *stores, serviceStore network.ServiceStore) *edgeServiceStoreImpl {
	store := &edgeServiceStoreImpl{
		baseStore: newChildBaseStore(stores, serviceStore, EntityTypeServices),
	}
	store.InitImpl(store)
	return store
}

type edgeServiceStoreImpl struct {
	*baseStore

	indexName           boltz.ReadIndex
	indexRoleAttributes boltz.SetReadIndex
	symbolAppwans       boltz.EntitySetSymbol
	symbolClusters      boltz.EntitySetSymbol
	symbolSessions      boltz.EntitySetSymbol

	symbolEdgeRouters      boltz.EntitySetSymbol
	symbolEdgeRoutersRoles boltz.EntitySetSymbol
	symbolServicePolicies  boltz.EntitySetSymbol
	symbolConfigs          boltz.EntitySetSymbol

	edgeRouterCollection boltz.LinkCollection
}

func (store *edgeServiceStoreImpl) NewStoreEntity() boltz.BaseEntity {
	return &EdgeService{}
}

func (store *edgeServiceStoreImpl) initializeLocal() {
	store.addBaseFields()
	store.GetParentStore().GrantSymbols(store)

	store.indexName = store.addUniqueNameField()
	store.indexRoleAttributes = store.addRoleAttributesField()

	store.AddSymbol(FieldServiceDnsHostname, ast.NodeTypeString)
	store.AddSymbol(FieldServiceDnsPort, ast.NodeTypeInt64)
	store.symbolAppwans = store.AddFkSetSymbol(FieldServiceAppwans, store.stores.appwan)
	store.symbolClusters = store.AddFkSetSymbol(FieldServiceClusters, store.stores.cluster)
	store.symbolSessions = store.AddFkSetSymbol(EntityTypeSessions, store.stores.session)
	store.symbolEdgeRoutersRoles = store.AddSetSymbol(FieldServiceEdgeRouterRoles, ast.NodeTypeString)
	store.symbolEdgeRouters = store.AddFkSetSymbol(EntityTypeEdgeRouters, store.stores.edgeRouter)
	store.symbolServicePolicies = store.AddFkSetSymbol(EntityTypeServicePolicies, store.stores.servicePolicy)
	store.symbolConfigs = store.AddFkSetSymbol(EntityTypeConfigs, store.stores.config)

	store.indexRoleAttributes.AddListener(store.rolesChanged)
}

func (store *edgeServiceStoreImpl) edgeRouterRolesChanged(ctx *boltz.PersistContext, entityId string, roles []string) {
	roleIds, err := store.getEntityIdsForRoleSet(ctx.Bucket.Tx(), "edgeRouterRoles", roles, store.stores.edgeRouter.indexRoleAttributes, store.stores.edgeRouter)
	if !ctx.Bucket.SetError(err) {
		ctx.Bucket.SetError(store.edgeRouterCollection.SetLinks(ctx.Bucket.Tx(), entityId, roleIds))
	}
}

func (store *edgeServiceStoreImpl) rolesChanged(tx *bbolt.Tx, rowId []byte, _ []boltz.FieldTypeAndValue, new []boltz.FieldTypeAndValue, holder errorz.ErrorHolder) {
	rolesSymbol := store.stores.servicePolicy.symbolServiceRoles
	linkCollection := store.stores.servicePolicy.serviceCollection
	UpdateRelatedRoles(store, tx, string(rowId), rolesSymbol, linkCollection, new, holder)
}

func (store *edgeServiceStoreImpl) nameChanged(bucket *boltz.TypedBucket, entity NamedEdgeEntity, oldName string) {
	store.updateEntityNameReferences(bucket, store.stores.servicePolicy.symbolServiceRoles, entity, oldName)
}

func (store *edgeServiceStoreImpl) initializeLinked() {
	store.AddLinkCollection(store.symbolAppwans, store.stores.appwan.symbolServices)
	store.AddLinkCollection(store.symbolClusters, store.stores.cluster.symbolServices)
	store.edgeRouterCollection = store.AddLinkCollection(store.symbolEdgeRouters, store.stores.edgeRouter.symbolServices)
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
	}

	return store.BaseStore.DeleteById(ctx, id)
}
