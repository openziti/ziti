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
	"fmt"
	"github.com/openziti/edge/eid"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/foundation/storage/ast"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/openziti/foundation/util/errorz"
	"github.com/sirupsen/logrus"
	"go.etcd.io/bbolt"
)

const (
	FieldEdgeRouters                     = "edgeRouters"
	FieldEdgeRouterCertPEM               = "certPem"
	FieldEdgeRouterUnverifiedCertPEM     = "unverifiedCertPem"
	FieldEdgeRouterUnverifiedFingerprint = "unverifiedFingerprint"
	FieldEdgeRouterIsVerified            = "isVerified"
	FieldEdgeRouterHostname              = "hostname"
	FieldEdgeRouterProtocols             = "protocols"
	FieldEdgeRouterEnrollments           = "enrollments"
	FieldEdgeRouterIsTunnelerEnabled     = "isTunnelerEnabled"
	FieldEdgeRouterAppData               = "appData"
)

func newEdgeRouter(name string, roleAttributes ...string) *EdgeRouter {
	return &EdgeRouter{
		Router: db.Router{
			BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
			Name:          name,
		},
		RoleAttributes: roleAttributes,
	}
}

type EdgeRouter struct {
	db.Router
	IsVerified            bool
	CertPem               *string
	UnverifiedCertPem     *string
	UnverifiedFingerprint *string
	Hostname              *string
	EdgeRouterProtocols   map[string]string
	RoleAttributes        []string
	Enrollments           []string
	IsTunnelerEnabled     bool
	AppData               map[string]interface{}
}

func (entity *EdgeRouter) LoadValues(store boltz.CrudStore, bucket *boltz.TypedBucket) {
	_, err := store.GetParentStore().BaseLoadOneById(bucket.Tx(), entity.Id, &entity.Router)
	bucket.SetError(err)

	entity.CertPem = bucket.GetString(FieldEdgeRouterCertPEM)
	entity.IsVerified = bucket.GetBoolWithDefault(FieldEdgeRouterIsVerified, false)
	entity.IsTunnelerEnabled = bucket.GetBoolWithDefault(FieldEdgeRouterIsTunnelerEnabled, entity.IsTunnelerEnabled)

	entity.UnverifiedFingerprint = bucket.GetString(FieldEdgeRouterUnverifiedFingerprint)
	entity.UnverifiedCertPem = bucket.GetString(FieldEdgeRouterUnverifiedCertPEM)

	//old v4, migrations only
	entity.Enrollments = bucket.GetStringList(FieldEdgeRouterEnrollments)
	entity.Hostname = bucket.GetString(FieldEdgeRouterHostname)
	entity.EdgeRouterProtocols = toStringStringMap(bucket.GetMap(FieldEdgeRouterProtocols))
	entity.RoleAttributes = bucket.GetStringList(FieldRoleAttributes)
	entity.AppData = bucket.GetMap(FieldIdentityAppData)
}

func (entity *EdgeRouter) SetValues(ctx *boltz.PersistContext) {
	entity.Router.SetValues(ctx.GetParentContext())

	store := ctx.Store.(*edgeRouterStoreImpl)
	ctx.SetStringP(FieldEdgeRouterCertPEM, entity.CertPem)
	ctx.SetBool(FieldEdgeRouterIsVerified, entity.IsVerified)
	ctx.SetStringP(FieldEdgeRouterHostname, entity.Hostname)
	ctx.SetMap(FieldEdgeRouterProtocols, toStringInterfaceMap(entity.EdgeRouterProtocols))
	store.validateRoleAttributes(entity.RoleAttributes, ctx.Bucket)
	ctx.SetStringList(FieldRoleAttributes, entity.RoleAttributes)
	ctx.SetBool(FieldEdgeRouterIsTunnelerEnabled, entity.IsTunnelerEnabled)
	ctx.Bucket.PutMap(FieldEdgeRouterAppData, entity.AppData, ctx.FieldChecker, false)

	ctx.SetStringP(FieldEdgeRouterUnverifiedFingerprint, entity.UnverifiedFingerprint)
	ctx.SetStringP(FieldEdgeRouterUnverifiedCertPEM, entity.UnverifiedCertPem)

	// index change won't fire if we don't have any roles on create, but we need to evaluate if we match any #all roles
	if ctx.IsCreate && len(entity.RoleAttributes) == 0 {
		store.rolesChanged(ctx.MutateContext, []byte(entity.Id), nil, nil, ctx.Bucket)
	}
}

func (entity *EdgeRouter) GetName() string {
	return entity.Name
}

type EdgeRouterStore interface {
	NameIndexedStore
	LoadOneById(tx *bbolt.Tx, id string) (*EdgeRouter, error)
	LoadOneByName(tx *bbolt.Tx, id string) (*EdgeRouter, error)
	GetRoleAttributesIndex() boltz.SetReadIndex
	GetRoleAttributesCursorProvider(values []string, semantic string) (ast.SetCursorProvider, error)
}

func newEdgeRouterStore(stores *stores) *edgeRouterStoreImpl {
	parentMapper := func(entity boltz.Entity) boltz.Entity {
		if edgeRouter, ok := entity.(*EdgeRouter); ok {
			return &edgeRouter.Router
		}
		return entity
	}

	store := &edgeRouterStoreImpl{}
	stores.Router.AddDeleteHandler(store.cleanupEdgeRouter) // do cleanup first
	store.baseStore = newChildBaseStore(stores, stores.Router, parentMapper)
	store.InitImpl(store)
	return store
}

type edgeRouterStoreImpl struct {
	*baseStore

	indexName           boltz.ReadIndex
	indexRoleAttributes boltz.SetReadIndex

	symbolRoleAttributes            boltz.EntitySetSymbol
	symbolEdgeRouterPolicies        boltz.EntitySetSymbol
	symbolServiceEdgeRouterPolicies boltz.EntitySetSymbol
	symbolEnrollments               boltz.EntitySetSymbol

	symbolIdentities boltz.EntitySetSymbol
	symbolServices   boltz.EntitySetSymbol

	identitiesCollection boltz.RefCountedLinkCollection
	servicesCollection   boltz.RefCountedLinkCollection
}

func (store *edgeRouterStoreImpl) NewStoreEntity() boltz.Entity {
	return &EdgeRouter{}
}

func (store *edgeRouterStoreImpl) GetRoleAttributesIndex() boltz.SetReadIndex {
	return store.indexRoleAttributes
}

func (store *edgeRouterStoreImpl) initializeLocal() {
	store.GetParentStore().GrantSymbols(store)

	store.symbolRoleAttributes = store.AddPublicSetSymbol(FieldRoleAttributes, ast.NodeTypeString)

	store.indexName = store.GetParentStore().(db.RouterStore).GetNameIndex()
	store.indexRoleAttributes = store.AddSetIndex(store.symbolRoleAttributes)

	store.AddSymbol(FieldEdgeRouterIsVerified, ast.NodeTypeBool)
	store.AddSymbol(FieldEdgeRouterIsTunnelerEnabled, ast.NodeTypeBool)

	store.symbolEnrollments = store.AddFkSetSymbol(FieldEdgeRouterEnrollments, store.stores.enrollment)
	store.symbolEdgeRouterPolicies = store.AddFkSetSymbol(EntityTypeEdgeRouterPolicies, store.stores.edgeRouterPolicy)
	store.symbolServiceEdgeRouterPolicies = store.AddFkSetSymbol(EntityTypeServiceEdgeRouterPolicies, store.stores.serviceEdgeRouterPolicy)

	store.symbolIdentities = store.AddFkSetSymbol(EntityTypeIdentities, store.stores.identity)
	store.symbolServices = store.AddFkSetSymbol(db.EntityTypeServices, store.stores.edgeService)

	store.indexRoleAttributes.AddListener(store.rolesChanged)
}

func (store *edgeRouterStoreImpl) initializeLinked() {
	store.AddLinkCollection(store.symbolEdgeRouterPolicies, store.stores.edgeRouterPolicy.symbolEdgeRouters)
	store.AddLinkCollection(store.symbolServiceEdgeRouterPolicies, store.stores.serviceEdgeRouterPolicy.symbolEdgeRouters)

	store.identitiesCollection = store.AddRefCountedLinkCollection(store.symbolIdentities, store.stores.identity.symbolEdgeRouters)
	store.servicesCollection = store.AddRefCountedLinkCollection(store.symbolServices, store.stores.edgeService.symbolEdgeRouters)

	store.AddConstraint(&routerIdentityConstraint{
		stores:                store.stores,
		routerNameSymbol:      store.GetSymbol(FieldName),
		tunnelerEnabledSymbol: store.GetSymbol(FieldEdgeRouterIsTunnelerEnabled),
	})
}

func (store *edgeRouterStoreImpl) rolesChanged(mutateCtx boltz.MutateContext, rowId []byte, _ []boltz.FieldTypeAndValue, new []boltz.FieldTypeAndValue, holder errorz.ErrorHolder) {
	// Recalculate edge router policy links
	ctx := &roleAttributeChangeContext{
		tx:                    mutateCtx.Tx(),
		rolesSymbol:           store.stores.edgeRouterPolicy.symbolEdgeRouterRoles,
		linkCollection:        store.stores.edgeRouterPolicy.edgeRouterCollection,
		relatedLinkCollection: store.stores.edgeRouterPolicy.identityCollection,
		denormLinkCollection:  store.identitiesCollection,
		ErrorHolder:           holder,
	}
	UpdateRelatedRoles(ctx, rowId, new, store.stores.edgeRouterPolicy.symbolSemantic)

	// Recalculate service edge router policy links
	ctx = &roleAttributeChangeContext{
		tx:                    mutateCtx.Tx(),
		rolesSymbol:           store.stores.serviceEdgeRouterPolicy.symbolEdgeRouterRoles,
		linkCollection:        store.stores.serviceEdgeRouterPolicy.edgeRouterCollection,
		relatedLinkCollection: store.stores.serviceEdgeRouterPolicy.serviceCollection,
		denormLinkCollection:  store.servicesCollection,
		ErrorHolder:           holder,
	}
	UpdateRelatedRoles(ctx, rowId, new, store.stores.serviceEdgeRouterPolicy.symbolSemantic)
}

func (store *edgeRouterStoreImpl) GetNameIndex() boltz.ReadIndex {
	return store.indexName
}

func (store *edgeRouterStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*EdgeRouter, error) {
	entity := &EdgeRouter{}
	if err := store.baseLoadOneById(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *edgeRouterStoreImpl) LoadOneByName(tx *bbolt.Tx, name string) (*EdgeRouter, error) {
	id := store.indexName.Read(tx, []byte(name))
	if id != nil {
		return store.LoadOneById(tx, string(id))
	}
	return nil, nil
}

func (store *edgeRouterStoreImpl) cleanupEdgeRouter(ctx boltz.MutateContext, id string) error {
	if entity, _ := store.LoadOneById(ctx.Tx(), id); entity != nil {
		// Remove entity from EdgeRouterRoles in edge router policies
		if err := store.deleteEntityReferences(ctx.Tx(), entity, store.stores.edgeRouterPolicy.symbolEdgeRouterRoles); err != nil {
			return err
		}
		// Remove entity from EdgeRouterRoles in service edge router policies
		if err := store.deleteEntityReferences(ctx.Tx(), entity, store.stores.serviceEdgeRouterPolicy.symbolEdgeRouterRoles); err != nil {
			return err
		}

		// Remove outstanding enrollments
		if err := store.stores.enrollment.DeleteWhere(ctx, fmt.Sprintf(`edgeRouter="%s"`, entity.Id)); err != nil {
			return err
		}
	}
	return nil
}

func (store *edgeRouterStoreImpl) GetRoleAttributesCursorProvider(values []string, semantic string) (ast.SetCursorProvider, error) {
	return store.getRoleAttributesCursorProvider(store.indexRoleAttributes, values, semantic)
}

type routerIdentityConstraint struct {
	stores                *stores
	routerNameSymbol      boltz.EntitySymbol
	tunnelerEnabledSymbol boltz.EntitySymbol
}

func (self *routerIdentityConstraint) ProcessBeforeUpdate(ctx *boltz.IndexingContext) {
	if !ctx.IsCreate {
		t, v := self.routerNameSymbol.Eval(ctx.Tx(), ctx.RowId)
		ctx.PushState(self, t, v)

		t, v = self.tunnelerEnabledSymbol.Eval(ctx.Tx(), ctx.RowId)
		ctx.PushState(self, t, v)
	}
}

func (self *routerIdentityConstraint) ProcessAfterUpdate(ctx *boltz.IndexingContext) {
	oldName := ctx.PopStateString(self)
	oldTunnelerEnabled := ctx.PopStateBool(self)
	_, currentName := self.routerNameSymbol.Eval(ctx.Tx(), ctx.RowId)
	currentTunnelerEnabledP := boltz.FieldToBool(self.tunnelerEnabledSymbol.Eval(ctx.Tx(), ctx.RowId))
	currentTunnelerEnabled := currentTunnelerEnabledP != nil && *currentTunnelerEnabledP
	name := string(currentName)

	createEntities := !oldTunnelerEnabled && currentTunnelerEnabled

	routerId := string(ctx.RowId)

	if createEntities {
		identity := &Identity{
			BaseExtEntity: boltz.BaseExtEntity{
				Id: string(ctx.RowId),
			},
			Name:           name,
			IdentityTypeId: RouterIdentityType,
			IsDefaultAdmin: false,
			IsAdmin:        false,
		}
		ctx.ErrHolder.SetError(self.stores.identity.Create(ctx.Ctx, identity))

		edgeRouterPolicy := &EdgeRouterPolicy{
			BaseExtEntity: boltz.BaseExtEntity{
				Id:       routerId,
				IsSystem: true,
			},
			Name:            getSystemEdgeRouterPolicyName(string(ctx.RowId)),
			Semantic:        "AnyOf",
			IdentityRoles:   []string{"@" + identity.Id},
			EdgeRouterRoles: []string{"@" + string(ctx.RowId)},
		}

		ctx.ErrHolder.SetError(self.stores.edgeRouterPolicy.Create(ctx.Ctx.GetSystemContext(), edgeRouterPolicy))
	} else if oldTunnelerEnabled && !currentTunnelerEnabled {
		self.deleteAssociatedEntities(ctx)
	}

	if !createEntities && currentTunnelerEnabled && oldName != name {
		identity, err := self.stores.identity.LoadOneById(ctx.Tx(), routerId)
		if err != nil {
			if boltz.IsErrNotFoundErr(err) {
				logrus.Errorf("identity for router with id %v not found", routerId)
			} else {
				ctx.ErrHolder.SetError(err)
			}
			return
		}

		if identity.IdentityTypeId != RouterIdentityType {
			logrus.Errorf("identity matching router with name %v is not a router identity, not updating name", oldName)
			return
		}

		identity.Name = name
		err = self.stores.identity.Update(ctx.Ctx, identity, boltz.MapFieldChecker{
			FieldName: struct{}{},
		})
		ctx.ErrHolder.SetError(err)
	}
}

func (self *routerIdentityConstraint) ProcessBeforeDelete(ctx *boltz.IndexingContext) {
	self.deleteAssociatedEntities(ctx)
}

func (self *routerIdentityConstraint) deleteAssociatedEntities(ctx *boltz.IndexingContext) {
	routerId := string(ctx.RowId)

	// cleanup associated auto-generated edge router policy
	if self.stores.edgeRouterPolicy.IsEntityPresent(ctx.Tx(), routerId) {
		ctx.ErrHolder.SetError(self.stores.edgeRouterPolicy.DeleteById(ctx.Ctx.GetSystemContext(), routerId))
	}

	if self.stores.identity.IsEntityPresent(ctx.Ctx.Tx(), routerId) {
		_, identityType := self.stores.identity.symbolIdentityTypeId.Eval(ctx.Tx(), ctx.RowId)
		if string(identityType) != RouterIdentityType {
			logrus.Debugf("identity matching router with id %v is not a router identity, not deleting", routerId)
			return
		}

		ctx.ErrHolder.SetError(self.stores.identity.DeleteById(ctx.Ctx.GetSystemContext(), routerId))
	}
}

func (self *routerIdentityConstraint) Initialize(_ *bbolt.Tx, _ errorz.ErrorHolder) {}

func (self *routerIdentityConstraint) CheckIntegrity(_ *bbolt.Tx, _ bool, _ func(err error, fixed bool)) error {
	return nil
}

func getSystemEdgeRouterPolicyName(edgeRouterId string) string {
	return "edge-router-" + edgeRouterId + "-system"
}
