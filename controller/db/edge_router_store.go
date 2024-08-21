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
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/common/eid"
	"github.com/sirupsen/logrus"
	"go.etcd.io/bbolt"
)

const (
	FieldEdgeRouters                     = "edgeRouters"
	FieldEdgeRouterCertPEM               = "certPem"
	FieldEdgeRouterUnverifiedCertPEM     = "unverifiedCertPem"
	FieldEdgeRouterUnverifiedFingerprint = "unverifiedFingerprint"
	FieldEdgeRouterIsVerified            = "isVerified"
	FieldEdgeRouterIsTunnelerEnabled     = "isTunnelerEnabled"
	FieldEdgeRouterAppData               = "appData"
)

func newEdgeRouter(name string, roleAttributes ...string) *EdgeRouter {
	return &EdgeRouter{
		Router: Router{
			BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
			Name:          name,
		},
		RoleAttributes: roleAttributes,
	}
}

type EdgeRouter struct {
	Router
	IsVerified            bool                   `json:"isVerified"`
	CertPem               *string                `json:"certPem"`
	UnverifiedCertPem     *string                `json:"unverifiedCertPem"`
	UnverifiedFingerprint *string                `json:"unverifiedFingerprint"`
	RoleAttributes        []string               `json:"roleAttributes"`
	IsTunnelerEnabled     bool                   `json:"isTunnelerEnabled"`
	AppData               map[string]interface{} `json:"appData"`
}

func (entity *EdgeRouter) GetName() string {
	return entity.Name
}

var _ EdgeRouterStore = (*edgeRouterStoreImpl)(nil)

type EdgeRouterStore interface {
	NameIndexed
	Store[*EdgeRouter]
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
	store.baseStore = newChildBaseStore[*EdgeRouter](stores, parentMapper, store, stores.router, EdgeBucket)
	store.InitImpl(store)

	stores.router.RegisterChildStoreStrategy(store) // do cleanup first
	return store
}

type edgeRouterStoreImpl struct {
	*baseStore[*EdgeRouter]

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

func (store *edgeRouterStoreImpl) HandleUpdate(ctx boltz.MutateContext, entity *Router, checker boltz.FieldChecker) (bool, error) {
	er, found, err := store.FindById(ctx.Tx(), entity.Id)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}

	er.Router = *entity
	return true, store.Update(ctx, er, checker)
}

func (store *edgeRouterStoreImpl) HandleDelete(ctx boltz.MutateContext, entity *Router) error {
	return store.cleanupEdgeRouter(ctx, entity.Id)
}

func (store *edgeRouterStoreImpl) GetStore() boltz.Store {
	return store
}

func (store *edgeRouterStoreImpl) GetRoleAttributesIndex() boltz.SetReadIndex {
	return store.indexRoleAttributes
}

func (store *edgeRouterStoreImpl) initializeLocal() {
	store.GetParentStore().GrantSymbols(store)

	store.symbolRoleAttributes = store.AddPublicSetSymbol(FieldRoleAttributes, ast.NodeTypeString)

	store.indexName = store.GetParentStore().(RouterStore).GetNameIndex()
	store.indexRoleAttributes = store.AddSetIndex(store.symbolRoleAttributes)

	store.AddSymbol(FieldEdgeRouterIsVerified, ast.NodeTypeBool)
	store.AddSymbol(FieldEdgeRouterIsTunnelerEnabled, ast.NodeTypeBool)

	store.symbolEnrollments = store.AddFkSetSymbol(EntityTypeEnrollments, store.stores.enrollment)
	store.symbolEdgeRouterPolicies = store.AddFkSetSymbol(EntityTypeEdgeRouterPolicies, store.stores.edgeRouterPolicy)
	store.symbolServiceEdgeRouterPolicies = store.AddFkSetSymbol(EntityTypeServiceEdgeRouterPolicies, store.stores.serviceEdgeRouterPolicy)

	store.symbolIdentities = store.AddFkSetSymbol(EntityTypeIdentities, store.stores.identity)
	store.symbolServices = store.AddFkSetSymbol(EntityTypeServices, store.stores.edgeService)

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

func (store *edgeRouterStoreImpl) NewEntity() *EdgeRouter {
	return &EdgeRouter{}
}

func (store *edgeRouterStoreImpl) FillEntity(entity *EdgeRouter, bucket *boltz.TypedBucket) {
	store.stores.router.FillEntity(&entity.Router, store.getParentBucket(entity, bucket))

	entity.CertPem = bucket.GetString(FieldEdgeRouterCertPEM)
	entity.IsVerified = bucket.GetBoolWithDefault(FieldEdgeRouterIsVerified, false)
	entity.IsTunnelerEnabled = bucket.GetBoolWithDefault(FieldEdgeRouterIsTunnelerEnabled, entity.IsTunnelerEnabled)

	entity.UnverifiedFingerprint = bucket.GetString(FieldEdgeRouterUnverifiedFingerprint)
	entity.UnverifiedCertPem = bucket.GetString(FieldEdgeRouterUnverifiedCertPEM)

	entity.RoleAttributes = bucket.GetStringList(FieldRoleAttributes)
	entity.AppData = bucket.GetMap(FieldEdgeRouterAppData)

}

func (store *edgeRouterStoreImpl) PersistEntity(entity *EdgeRouter, ctx *boltz.PersistContext) {
	store.stores.router.PersistEntity(&entity.Router, ctx.GetParentContext())

	ctx.SetStringP(FieldEdgeRouterCertPEM, entity.CertPem)
	ctx.SetBool(FieldEdgeRouterIsVerified, entity.IsVerified)
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

func (store *edgeRouterStoreImpl) rolesChanged(mutateCtx boltz.MutateContext, rowId []byte, _ []boltz.FieldTypeAndValue, new []boltz.FieldTypeAndValue, holder errorz.ErrorHolder) {
	// Recalculate edge router policy links
	ctx := &roleAttributeChangeContext{
		mutateCtx:             mutateCtx,
		rolesSymbol:           store.stores.edgeRouterPolicy.symbolEdgeRouterRoles,
		linkCollection:        store.stores.edgeRouterPolicy.edgeRouterCollection,
		relatedLinkCollection: store.stores.edgeRouterPolicy.identityCollection,
		denormLinkCollection:  store.identitiesCollection,
		ErrorHolder:           holder,
	}
	UpdateRelatedRoles(ctx, rowId, new, store.stores.edgeRouterPolicy.symbolSemantic)

	// Recalculate service edge router policy links
	ctx = &roleAttributeChangeContext{
		mutateCtx:             mutateCtx,
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

func (store *edgeRouterStoreImpl) cleanupEdgeRouter(ctx boltz.MutateContext, id string) error {
	if entity, _ := store.LoadById(ctx.Tx(), id); entity != nil {
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

func (index *routerIdentityConstraint) Label() string {
	return "router identity constraint"
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
		identity, err := self.stores.identity.LoadById(ctx.Tx(), routerId)
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

func (self *routerIdentityConstraint) CheckIntegrity(_ boltz.MutateContext, _ bool, _ func(err error, fixed bool)) error {
	return nil
}

func getSystemEdgeRouterPolicyName(edgeRouterId string) string {
	return "edge-router-" + edgeRouterId + "-system"
}
