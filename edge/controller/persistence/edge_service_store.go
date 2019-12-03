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
	"github.com/netfoundry/ziti-fabric/fabric/controller/network"
	"github.com/netfoundry/ziti-foundation/storage/ast"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/michaelquigley/pfxlog"
	"go.etcd.io/bbolt"
	"reflect"
)

type EdgeService struct {
	network.Service
	EdgeEntityFields
	Name        string
	DnsHostname string
	DnsPort     uint16
	AppWans     []string
	Clusters    []string
	HostIds     []string
}

const (
	FieldServiceDnsHostname       = "dnsHostname"
	FieldServiceDnsPort           = "dnsPort"
	FieldServiceAppwans           = "appwans"
	FieldServiceClusters          = "clusters"
	FieldServiceSessions          = "sessions"
	FieldServiceHostingIdentities = "hostingIdentities"
)

func (entity *EdgeService) LoadValues(store boltz.CrudStore, bucket *boltz.TypedBucket) {
	_, err := store.GetParentStore().BaseLoadOneById(bucket.Tx(), entity.Id, &entity.Service)
	bucket.SetError(err)

	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
	entity.DnsHostname = bucket.GetStringWithDefault(FieldServiceDnsHostname, "")
	entity.DnsPort = uint16(bucket.GetInt32WithDefault(FieldServiceDnsPort, 0))
	entity.AppWans = bucket.GetStringList(FieldServiceAppwans)
	entity.Clusters = bucket.GetStringList(FieldServiceClusters)
	entity.HostIds = bucket.GetStringList(FieldServiceHostingIdentities)
}

var edgeServiceFieldMappings = map[string]string{FieldServiceHostingIdentities: "hostIds"}

func (entity *EdgeService) SetValues(ctx *boltz.PersistContext) {
	entity.Service.SetValues(ctx.GetParentContext())

	ctx.WithFieldOverrides(edgeServiceFieldMappings)
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldName, entity.Name)
	ctx.SetString(FieldServiceDnsHostname, entity.DnsHostname)
	ctx.SetInt32(FieldServiceDnsPort, int32(entity.DnsPort))

	if ctx.IsCreate {
		ctx.SetLinkedIds(FieldServiceClusters, entity.Clusters)
		ctx.SetLinkedIds(FieldServiceHostingIdentities, entity.HostIds)
	}
}

func (entity *EdgeService) GetEntityType() string {
	return EntityTypeServices
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

	indexName      boltz.ReadIndex
	symbolAppwans  boltz.EntitySetSymbol
	symbolClusters boltz.EntitySetSymbol
	symbolHostIds  boltz.EntitySetSymbol
	symbolSessions boltz.EntitySetSymbol
}

func (store *edgeServiceStoreImpl) NewStoreEntity() boltz.BaseEntity {
	return &EdgeService{}
}

func (store *edgeServiceStoreImpl) initializeLocal() {
	store.addBaseFields()
	store.GetParentStore().GrantSymbols(store)

	store.indexName = store.addUniqueNameField()
	store.AddSymbol(FieldServiceDnsHostname, ast.NodeTypeString)
	store.AddSymbol(FieldServiceDnsPort, ast.NodeTypeInt64)
	store.symbolAppwans = store.AddFkSetSymbol(FieldServiceAppwans, store.stores.appwan)
	store.symbolClusters = store.AddFkSetSymbol(FieldServiceClusters, store.stores.cluster)
	store.symbolHostIds = store.AddFkSetSymbol(FieldServiceHostingIdentities, store.stores.identity)
	store.symbolSessions = store.AddFkSetSymbol(FieldServiceSessions, store.stores.session)
}

func (store *edgeServiceStoreImpl) initializeLinked() {
	store.AddLinkCollection(store.symbolAppwans, store.stores.appwan.symbolServices)
	store.AddLinkCollection(store.symbolClusters, store.stores.cluster.symbolServices)
	store.AddLinkCollection(store.symbolHostIds, store.stores.identity.symbolHostableServices)

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

func (store *edgeServiceStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*EdgeService, error) {
	service := &EdgeService{}
	if found, err := store.BaseLoadOneById(tx, id, service); !found || err != nil {
		return nil, err
	}
	return service, nil
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

func (store *edgeServiceStoreImpl) CleanupExternal(ctx boltz.MutateContext, id string) error {
	for _, sessionId := range store.GetRelatedEntitiesIdList(ctx.Tx(), id, FieldServiceSessions) {
		if err := store.stores.session.DeleteById(ctx, sessionId); err != nil {
			return err
		}
	}
	return store.baseStore.CleanupExternal(ctx, id)
}
