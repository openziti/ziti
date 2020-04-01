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
	"github.com/netfoundry/ziti-fabric/controller/db"
	"github.com/netfoundry/ziti-foundation/storage/ast"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/netfoundry/ziti-foundation/util/errorz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
)

type Stores struct {
	DbProvider DbProvider

	// fabric stores
	Router     db.RouterStore
	Service    db.ServiceStore
	Terminator db.TerminatorStore

	ApiSession              ApiSessionStore
	Appwan                  AppwanStore
	Ca                      CaStore
	Cluster                 ClusterStore
	Config                  ConfigStore
	ConfigType              ConfigTypeStore
	EdgeRouter              EdgeRouterStore
	EdgeRouterPolicy        EdgeRouterPolicyStore
	EdgeService             EdgeServiceStore
	EventLog                EventLogStore
	GeoRegion               GeoRegionStore
	Identity                IdentityStore
	IdentityType            IdentityTypeStore
	Index                   boltz.ListStore
	Session                 SessionStore
	ServiceEdgeRouterPolicy ServiceEdgeRouterPolicyStore
	ServicePolicy           ServicePolicyStore
	TransitRouter           TransitRouterStore
	Enrollment              EnrollmentStore
	Authenticator           AuthenticatorStore

	storeList []Store
	storeMap  map[string]boltz.CrudStore
}

func (stores *Stores) buildStoreMap() {
	val := reflect.ValueOf(stores).Elem()
	for i := 0; i < val.NumField(); i++ {
		f := val.Field(i)
		if f.CanInterface() {
			if store, ok := f.Interface().(boltz.CrudStore); ok {
				stores.storeMap[store.GetEntityType()] = store
			}
		}
	}
}

func (stores *Stores) getStoreList() []Store {
	var result []Store
	for _, crudStore := range stores.storeMap {
		if store, ok := crudStore.(Store); ok {
			result = append(result, store)
		}
	}
	return result
}

func (stores *Stores) GetStoreForEntity(entity boltz.Entity) boltz.CrudStore {
	return stores.storeMap[entity.GetEntityType()]
}

type stores struct {
	DbProvider DbProvider

	// fabric stores
	Router     db.RouterStore
	Service    db.ServiceStore
	Terminator db.TerminatorStore

	apiSession              *apiSessionStoreImpl
	appwan                  *appwanStoreImpl
	ca                      *caStoreImpl
	cluster                 *clusterStoreImpl
	config                  *configStoreImpl
	configType              *configTypeStoreImpl
	edgeRouter              *edgeRouterStoreImpl
	edgeRouterPolicy        *edgeRouterPolicyStoreImpl
	edgeService             *edgeServiceStoreImpl
	eventLog                *eventLogStoreImpl
	geoRegion               *geoRegionStoreImpl
	identity                *identityStoreImpl
	identityType            *IdentityTypeStoreImpl
	serviceEdgeRouterPolicy *serviceEdgeRouterPolicyStoreImpl
	servicePolicy           *servicePolicyStoreImpl
	session                 *sessionStoreImpl
	transitRouter           *transitRouterStoreImpl
	enrollment              *enrollmentStoreImpl
	authenticator           *authenticatorStoreImpl
}

func NewBoltStores(dbProvider DbProvider) (*Stores, error) {
	errorHolder := &errorz.ErrorHolderImpl{}

	internalStores := &stores{
		DbProvider: dbProvider,
	}

	internalStores.Terminator = dbProvider.GetStores().Terminator
	internalStores.Router = dbProvider.GetStores().Router
	internalStores.Service = dbProvider.GetStores().Service

	internalStores.apiSession = newApiSessionStore(internalStores)
	internalStores.authenticator = newAuthenticatorStore(internalStores)
	internalStores.appwan = newAppwanStore(internalStores)
	internalStores.ca = newCaStore(internalStores)
	internalStores.cluster = newClusterStore(internalStores)
	internalStores.config = newConfigsStore(internalStores)
	internalStores.configType = newConfigTypesStore(internalStores)
	internalStores.edgeRouter = newEdgeRouterStore(internalStores)
	internalStores.edgeRouterPolicy = newEdgeRouterPolicyStore(internalStores)
	internalStores.edgeService = newEdgeServiceStore(internalStores)
	internalStores.eventLog = newEventLogStore(internalStores)
	internalStores.transitRouter = newTransitRouterStore(internalStores)
	internalStores.geoRegion = newGeoRegionStore(internalStores)
	internalStores.identity = newIdentityStore(internalStores)
	internalStores.identityType = newIdentityTypeStore(internalStores)
	internalStores.enrollment = newEnrollmentStore(internalStores)
	internalStores.serviceEdgeRouterPolicy = newServiceEdgeRouterPolicyStore(internalStores)
	internalStores.servicePolicy = newServicePolicyStore(internalStores)
	internalStores.session = newSessionStore(internalStores)

	externalStores := &Stores{
		DbProvider: dbProvider,

		Terminator: dbProvider.GetStores().Terminator,
		Router:     dbProvider.GetStores().Router,
		Service:    dbProvider.GetStores().Service,

		ApiSession:              internalStores.apiSession,
		Appwan:                  internalStores.appwan,
		Ca:                      internalStores.ca,
		Cluster:                 internalStores.cluster,
		Config:                  internalStores.config,
		ConfigType:              internalStores.configType,
		EdgeRouter:              internalStores.edgeRouter,
		EdgeRouterPolicy:        internalStores.edgeRouterPolicy,
		EdgeService:             internalStores.edgeService,
		EventLog:                internalStores.eventLog,
		TransitRouter:           internalStores.transitRouter,
		GeoRegion:               internalStores.geoRegion,
		Identity:                internalStores.identity,
		IdentityType:            internalStores.identityType,
		ServiceEdgeRouterPolicy: internalStores.serviceEdgeRouterPolicy,
		ServicePolicy:           internalStores.servicePolicy,
		Session:                 internalStores.session,
		Authenticator:           internalStores.authenticator,
		Enrollment:              internalStores.enrollment,

		storeMap: make(map[string]boltz.CrudStore),
	}

	// The Index store is used for querying indexes. It's a convenient store with only a single value (id), which
	// is only ever queried using an index set cursor
	externalStores.Index = boltz.NewBaseStore(nil, "invalid", func(id string) error {
		return errors.Errorf("should never happen")
	})
	externalStores.Index.AddIdSymbol("id", ast.NodeTypeString)

	externalStores.buildStoreMap()
	storeList := externalStores.getStoreList()

	err := dbProvider.GetDb().Update(func(tx *bbolt.Tx) error {
		for _, store := range storeList {
			store.initializeLocal()
		}
		for _, store := range storeList {
			store.initializeLinked()
		}
		for _, store := range storeList {
			store.initializeIndexes(tx, errorHolder)
		}
		return nil
	})

	errorHolder.SetError(err)
	if errorHolder.HasError() {
		return nil, errorHolder.GetError()
	}
	return externalStores, nil
}

type QueryContext struct {
	Stores *Stores
	Tx     *bbolt.Tx
}
