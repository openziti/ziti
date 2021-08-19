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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/foundation/storage/ast"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/openziti/foundation/util/errorz"
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
	ApiSessionCertificate   ApiSessionCertificateStore
	Ca                      CaStore
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
	PostureCheck            PostureCheckStore
	PostureCheckType        PostureCheckTypeStore
	Mfa                     MfaStore
	storeMap                map[reflect.Type]boltz.CrudStore
}

func (stores *Stores) buildStoreMap() {
	val := reflect.ValueOf(stores).Elem()
	for i := 0; i < val.NumField(); i++ {
		f := val.Field(i)
		if f.CanInterface() {
			if store, ok := f.Interface().(boltz.CrudStore); ok {
				entityType := reflect.TypeOf(store.NewStoreEntity())
				stores.storeMap[entityType] = store
			}
		}
	}
}

func (stores *Stores) GetStoreList() []Store {
	var result []Store
	for _, crudStore := range stores.storeMap {
		if store, ok := crudStore.(Store); ok {
			result = append(result, store)
		}
	}
	return result
}

func (stores *Stores) GetStoreForEntity(entity boltz.Entity) boltz.CrudStore {
	return stores.storeMap[reflect.TypeOf(entity)]
}

func (stores *Stores) CheckIntegrity(fix bool, errorHandler func(error, bool)) error {
	if fix {
		return stores.DbProvider.GetDb().Update(func(tx *bbolt.Tx) error {
			return stores.CheckIntegrityInTx(tx, fix, errorHandler)
		})
	}

	return stores.DbProvider.GetDb().View(func(tx *bbolt.Tx) error {
		return stores.CheckIntegrityInTx(tx, fix, errorHandler)
	})
}

func (stores *Stores) CheckIntegrityInTx(tx *bbolt.Tx, fix bool, errorHandler func(error, bool)) error {
	if fix {
		pfxlog.Logger().Info("creating database snapshot before attempting to fix data integrity issues")
		if err := stores.DbProvider.GetDb().Snapshot(tx); err != nil {
			return err
		}
	}

	for _, store := range stores.storeMap {
		if err := store.CheckIntegrity(tx, fix, errorHandler); err != nil {
			return err
		}
	}
	return nil
}

type stores struct {
	DbProvider DbProvider

	// fabric stores
	Router     db.RouterStore
	Service    db.ServiceStore
	Terminator db.TerminatorStore

	apiSession              *apiSessionStoreImpl
	ca                      *caStoreImpl
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
	postureCheck            *postureCheckStoreImpl
	postureCheckType        *postureCheckTypeStoreImpl
	apiSessionCertificate   *ApiSessionCertificateStoreImpl
	mfa                     *MfaStoreImpl
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
	internalStores.apiSessionCertificate = newApiSessionCertificateStore(internalStores)
	internalStores.authenticator = newAuthenticatorStore(internalStores)
	internalStores.ca = newCaStore(internalStores)
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
	internalStores.postureCheck = newPostureCheckStore(internalStores)
	internalStores.postureCheckType = newPostureCheckTypeStore(internalStores)
	internalStores.mfa = newMfaStore(internalStores)

	externalStores := &Stores{
		DbProvider: dbProvider,

		Terminator: dbProvider.GetStores().Terminator,
		Router:     dbProvider.GetStores().Router,
		Service:    dbProvider.GetStores().Service,

		ApiSession:              internalStores.apiSession,
		ApiSessionCertificate:   internalStores.apiSessionCertificate,
		Ca:                      internalStores.ca,
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
		PostureCheck:            internalStores.postureCheck,
		PostureCheckType:        internalStores.postureCheckType,
		Mfa:                     internalStores.mfa,

		storeMap: make(map[reflect.Type]boltz.CrudStore),
	}

	// The Index store is used for querying indexes. It's a convenient store with only a single value (id), which
	// is only ever queried using an index set cursor
	externalStores.Index = boltz.NewBaseStore("invalid", func(id string) error {
		return errors.Errorf("should never happen")
	})
	externalStores.Index.AddIdSymbol("id", ast.NodeTypeString)

	externalStores.buildStoreMap()
	storeList := externalStores.GetStoreList()

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
