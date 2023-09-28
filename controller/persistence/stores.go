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

package persistence

import (
	"github.com/openziti/fabric/controller/change"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
	"time"
)

type Stores struct {
	DbProvider      DbProvider
	EventualEventer EventualEventer
	internal        *stores

	// fabric stores
	Router     db.RouterStore
	Service    db.ServiceStore
	Terminator db.TerminatorStore

	ApiSession              ApiSessionStore
	ApiSessionCertificate   ApiSessionCertificateStore
	AuthPolicy              AuthPolicyStore
	EventualEvent           EventualEventStore
	ExternalJwtSigner       ExternalJwtSignerStore
	Ca                      CaStore
	Config                  ConfigStore
	ConfigType              ConfigTypeStore
	EdgeRouter              EdgeRouterStore
	EdgeRouterPolicy        EdgeRouterPolicyStore
	EdgeService             EdgeServiceStore
	Identity                IdentityStore
	IdentityType            IdentityTypeStore
	Index                   boltz.Store
	Session                 SessionStore
	Revocation              RevocationStore
	ServiceEdgeRouterPolicy ServiceEdgeRouterPolicyStore
	ServicePolicy           ServicePolicyStore
	TransitRouter           TransitRouterStore
	Enrollment              EnrollmentStore
	Authenticator           AuthenticatorStore
	PostureCheck            PostureCheckStore
	PostureCheckType        PostureCheckTypeStore
	Mfa                     MfaStore
	storeMap                map[reflect.Type]boltz.Store
}

func (stores *Stores) addStoresToIntegrityCheck(fabricStores *db.Stores) {
	val := reflect.ValueOf(stores).Elem()
	for i := 0; i < val.NumField(); i++ {
		f := val.Field(i)
		if f.CanInterface() {
			// filter by the edge Store interface, so we don't recheck fabric stores, which are already being checked
			if store, ok := f.Interface().(boltz.Store); ok {
				fabricStores.AddCheckable(store)
			}
		}
	}
}

func (stores *Stores) buildStoreMap() {
	val := reflect.ValueOf(stores).Elem()
	for i := 0; i < val.NumField(); i++ {
		f := val.Field(i)
		if f.CanInterface() {
			if store, ok := f.Interface().(boltz.Store); ok && store.GetEntityType() != "indexes" {
				entityType := store.GetEntityReflectType()
				stores.storeMap[entityType] = store
			}
		}
	}
}

func (stores *Stores) GetEntityCounts(dbProvider DbProvider) (map[string]int64, error) {
	result := map[string]int64{}
	for _, store := range stores.storeMap {
		err := dbProvider.GetDb().View(func(tx *bbolt.Tx) error {
			key := store.GetEntityType()
			if store.IsChildStore() {
				if _, ok := store.(TransitRouterStore); ok {
					// skip transit routers, since count will be == fabric routers
					return nil
				} else {
					key = store.GetEntityType() + ".edge"
				}
			}

			_, count, err := store.QueryIds(tx, "true limit 1")
			if err != nil {
				return err
			}
			result[key] = count
			return nil
		})

		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (stores *Stores) getStoresForInit() []initializableStore {
	var result []initializableStore
	for _, crudStore := range stores.storeMap {
		if store, ok := crudStore.(initializableStore); ok {
			result = append(result, store)
		}
	}
	return result
}

func (stores *Stores) GetStoreForEntity(entity boltz.Entity) boltz.Store {
	return stores.storeMap[reflect.TypeOf(entity)]
}

func (stores *Stores) GetStores() []boltz.Store {
	var result []boltz.Store
	for _, store := range stores.storeMap {
		result = append(result, store)
	}
	return result
}

type stores struct {
	DbProvider      DbProvider
	EventualEventer EventualEventer

	// fabric stores
	Router     db.RouterStore
	Service    db.ServiceStore
	Terminator db.TerminatorStore

	apiSession              *apiSessionStoreImpl
	authPolicy              *AuthPolicyStoreImpl
	eventualEvent           *eventualEventStoreImpl
	ca                      *caStoreImpl
	config                  *configStoreImpl
	configType              *configTypeStoreImpl
	edgeRouter              *edgeRouterStoreImpl
	edgeRouterPolicy        *edgeRouterPolicyStoreImpl
	edgeService             *edgeServiceStoreImpl
	externalJwtSigner       *externalJwtSignerStoreImpl
	identity                *identityStoreImpl
	identityType            *IdentityTypeStoreImpl
	revocation              *revocationStoreImpl
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

	internalStores.eventualEvent = newEventualEventStore(internalStores)
	internalStores.EventualEventer = NewEventualEventerBbolt(dbProvider, internalStores.eventualEvent, 2*time.Second, 1000)

	internalStores.Terminator = dbProvider.GetStores().Terminator
	internalStores.Router = dbProvider.GetStores().Router
	internalStores.Service = dbProvider.GetStores().Service
	internalStores.apiSession = newApiSessionStore(internalStores)
	internalStores.apiSessionCertificate = newApiSessionCertificateStore(internalStores)
	internalStores.authenticator = newAuthenticatorStore(internalStores)
	internalStores.authPolicy = newAuthPolicyStore(internalStores)
	internalStores.ca = newCaStore(internalStores)
	internalStores.config = newConfigsStore(internalStores)
	internalStores.configType = newConfigTypesStore(internalStores)
	internalStores.edgeRouter = newEdgeRouterStore(internalStores)
	internalStores.edgeRouterPolicy = newEdgeRouterPolicyStore(internalStores)
	internalStores.edgeService = newEdgeServiceStore(internalStores)
	internalStores.externalJwtSigner = newExternalJwtSignerStore(internalStores)
	internalStores.transitRouter = newTransitRouterStore(internalStores)
	internalStores.identity = newIdentityStore(internalStores)
	internalStores.identityType = newIdentityTypeStore(internalStores)
	internalStores.enrollment = newEnrollmentStore(internalStores)
	internalStores.revocation = newRevocationStore(internalStores)
	internalStores.serviceEdgeRouterPolicy = newServiceEdgeRouterPolicyStore(internalStores)
	internalStores.servicePolicy = newServicePolicyStore(internalStores)
	internalStores.session = newSessionStore(internalStores)
	internalStores.postureCheck = newPostureCheckStore(internalStores)
	internalStores.postureCheckType = newPostureCheckTypeStore(internalStores)
	internalStores.mfa = newMfaStore(internalStores)

	externalStores := &Stores{
		DbProvider: dbProvider,
		internal:   internalStores,

		Terminator: dbProvider.GetStores().Terminator,
		Router:     dbProvider.GetStores().Router,
		Service:    dbProvider.GetStores().Service,

		ApiSession:              internalStores.apiSession,
		ApiSessionCertificate:   internalStores.apiSessionCertificate,
		AuthPolicy:              internalStores.authPolicy,
		EventualEvent:           internalStores.eventualEvent,
		Ca:                      internalStores.ca,
		Config:                  internalStores.config,
		ConfigType:              internalStores.configType,
		EdgeRouter:              internalStores.edgeRouter,
		EdgeRouterPolicy:        internalStores.edgeRouterPolicy,
		EdgeService:             internalStores.edgeService,
		ExternalJwtSigner:       internalStores.externalJwtSigner,
		TransitRouter:           internalStores.transitRouter,
		Identity:                internalStores.identity,
		IdentityType:            internalStores.identityType,
		Revocation:              internalStores.revocation,
		ServiceEdgeRouterPolicy: internalStores.serviceEdgeRouterPolicy,
		ServicePolicy:           internalStores.servicePolicy,
		Session:                 internalStores.session,
		Authenticator:           internalStores.authenticator,
		Enrollment:              internalStores.enrollment,
		PostureCheck:            internalStores.postureCheck,
		PostureCheckType:        internalStores.postureCheckType,
		Mfa:                     internalStores.mfa,

		storeMap: make(map[reflect.Type]boltz.Store),
	}

	externalStores.EventualEventer = internalStores.EventualEventer

	// The Index store is used for querying indexes. It's a convenient store with only a single value (id), which
	// is only ever queried using an index set cursor
	indexStoreDef := boltz.StoreDefinition[boltz.ExtEntity]{
		EntityType: "indexes",
		BasePath:   []string{db.RootBucket},
		EntityNotFoundF: func(id string) error {
			panic(errors.New("programming error"))
		},
	}

	indexStore := boltz.NewBaseStore(indexStoreDef)
	indexStore.AddIdSymbol("id", ast.NodeTypeString)

	externalStores.Index = indexStore

	externalStores.buildStoreMap()
	storeList := externalStores.getStoresForInit()

	mutateCtx := change.New().SetSourceType("system.initialization").SetChangeAuthorType(change.AuthorTypeController).NewMutateContext()
	err := dbProvider.GetDb().Update(mutateCtx, func(ctx boltz.MutateContext) error {
		for _, store := range storeList {
			store.initializeLocal()
		}
		for _, store := range storeList {
			store.initializeLinked()
		}
		for _, store := range storeList {
			store.initializeIndexes(ctx.Tx(), errorHolder)
		}
		return nil
	})

	externalStores.addStoresToIntegrityCheck(dbProvider.GetStores())

	errorHolder.SetError(err)
	if errorHolder.HasError() {
		return nil, errorHolder.GetError()
	}
	return externalStores, nil
}

func newBaseStore[E boltz.ExtEntity](stores *stores, strategy boltz.EntityStrategy[E]) *baseStore[E] {
	return &baseStore[E]{
		stores:    stores,
		BaseStore: boltz.NewBaseStore(db.NewStoreDefinition[E](strategy)),
	}
}

func newChildBaseStore[E boltz.ExtEntity](stores *stores, parentMapper func(entity boltz.Entity) boltz.Entity, strategy boltz.EntityStrategy[E], parent boltz.Store, path string) *baseStore[E] {
	def := db.NewStoreDefinition[E](strategy)
	def.BasePath = []string{path}
	def.Parent = parent
	def.ParentMapper = parentMapper
	return &baseStore[E]{
		stores:    stores,
		BaseStore: boltz.NewBaseStore[E](def),
	}
}
