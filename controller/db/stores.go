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
	"context"
	"errors"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/controller/change"
	"go.etcd.io/bbolt"
	"go4.org/sort"
	"reflect"
	"sync"
	"time"
)

func NewStoreDefinition[E boltz.ExtEntity](strategy boltz.EntityStrategy[E]) boltz.StoreDefinition[E] {
	entityType := strategy.NewEntity().GetEntityType()
	return boltz.StoreDefinition[E]{
		EntityType:     entityType,
		EntityStrategy: strategy,
		BasePath:       []string{RootBucket},
		EntityNotFoundF: func(id string) error {
			return boltz.NewNotFoundError(boltz.GetSingularEntityType(entityType), "id", id)
		},
	}
}

func (store *Stores) AddCheckable(checkable boltz.Checkable) {
	store.lock.Lock()
	defer store.lock.Unlock()
	store.checkables = append(store.checkables, checkable)
}

func (stores *Stores) GetStoreList() []boltz.Store {
	var result []boltz.Store
	for _, store := range stores.storeMap {
		result = append(result, store)
	}
	return result
}

func (stores *Stores) CheckIntegrity(db boltz.Db, ctx context.Context, fix bool, errorHandler func(error, bool)) error {
	if fix {
		changeCtx := boltz.NewMutateContext(ctx)
		return db.Update(changeCtx, func(changeCtx boltz.MutateContext) error {
			return stores.CheckIntegrityInTx(db, changeCtx, fix, errorHandler)
		})
	}

	return db.View(func(tx *bbolt.Tx) error {
		changeCtx := boltz.NewTxMutateContext(ctx, tx)
		return stores.CheckIntegrityInTx(db, changeCtx, fix, errorHandler)
	})
}

func (stores *Stores) CheckIntegrityInTx(db boltz.Db, ctx boltz.MutateContext, fix bool, errorHandler func(error, bool)) error {
	if fix {
		pfxlog.Logger().Info("creating database snapshot before attempting to fix data integrity issues")
		if err := db.Snapshot(ctx.Tx()); err != nil {
			return err
		}
	}

	for _, checkable := range stores.checkables {
		if err := checkable.CheckIntegrity(ctx, fix, errorHandler); err != nil {
			return err
		}
	}
	return nil
}

type Stores struct {
	EventualEventer EventualEventer
	internal        *stores

	Router                  RouterStore
	Service                 ServiceStore
	Terminator              TerminatorStore
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
	lock                    sync.Mutex
	checkables              []boltz.Checkable
}

func (stores *Stores) buildStoreMap() {
	stores.storeMap = map[reflect.Type]boltz.Store{}
	val := reflect.ValueOf(stores).Elem()
	for i := 0; i < val.NumField(); i++ {
		f := val.Field(i)
		if f.CanInterface() {
			if store, ok := f.Interface().(boltz.Store); ok && store.GetEntityType() != "indexes" {
				entityType := store.GetEntityReflectType()
				stores.storeMap[entityType] = store
				stores.AddCheckable(store)
			}
		}
	}
}

func (stores *Stores) GetEntityCounts(db boltz.Db) (map[string]int64, error) {
	result := map[string]int64{}
	for _, store := range stores.storeMap {
		err := db.View(func(tx *bbolt.Tx) error {
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
	key := reflect.TypeOf(entity)
	return stores.storeMap[key]
}

func (stores *Stores) GetStores() []boltz.Store {
	var result []boltz.Store
	for _, store := range stores.storeMap {
		result = append(result, store)
	}
	return result
}

type stores struct {
	EventualEventer EventualEventer

	terminator              *terminatorStoreImpl
	router                  *routerStoreImpl
	service                 *serviceStoreImpl
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

type DbProvider interface {
	GetDb() boltz.Db
}

type DbProviderF func() boltz.Db

func (f DbProviderF) GetDb() boltz.Db {
	return f()
}

func InitStores(db boltz.Db) (*Stores, error) {
	dbProvider := DbProviderF(func() boltz.Db {
		return db
	})
	errorHolder := &errorz.ErrorHolderImpl{}

	internalStores := &stores{}

	internalStores.eventualEvent = newEventualEventStore(internalStores)
	internalStores.EventualEventer = NewEventualEventerBbolt(dbProvider, internalStores.eventualEvent, 2*time.Second, 1000)

	internalStores.router = newRouterStore(internalStores)
	internalStores.service = newServiceStore(internalStores)
	internalStores.terminator = newTerminatorStore(internalStores)

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
		internal: internalStores,

		Terminator: internalStores.terminator,
		Router:     internalStores.router,
		Service:    internalStores.service,

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
		BasePath:   []string{RootBucket},
		EntityNotFoundF: func(id string) error {
			panic(errors.New("programming error"))
		},
	}

	indexStore := boltz.NewBaseStore(indexStoreDef)
	indexStore.AddIdSymbol("id", ast.NodeTypeString)

	externalStores.Index = indexStore

	externalStores.buildStoreMap()
	storeList := externalStores.getStoresForInit()

	sort.Slice(storeList, func(i, j int) bool {
		if storeList[i].IsChildStore() == storeList[j].IsChildStore() {
			return storeList[i].GetEntityType() < storeList[j].GetEntityType()
		}
		return !storeList[i].IsChildStore()
	})

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

	errorHolder.SetError(err)
	if errorHolder.HasError() {
		return nil, errorHolder.GetError()
	}

	if err = RunMigrations(db, externalStores); err != nil {
		return nil, err
	}

	return externalStores, nil
}

func newBaseStore[E boltz.ExtEntity](stores *stores, strategy boltz.EntityStrategy[E]) *baseStore[E] {
	return &baseStore[E]{
		stores:    stores,
		BaseStore: boltz.NewBaseStore(NewStoreDefinition[E](strategy)),
	}
}

func newChildBaseStore[E boltz.ExtEntity](stores *stores, parentMapper func(entity boltz.Entity) boltz.Entity, strategy boltz.EntityStrategy[E], parent boltz.Store, path string) *baseStore[E] {
	def := NewStoreDefinition[E](strategy)
	def.BasePath = []string{path}
	def.Parent = parent
	def.ParentMapper = parentMapper
	return &baseStore[E]{
		stores:    stores,
		BaseStore: boltz.NewBaseStore[E](def),
	}
}
