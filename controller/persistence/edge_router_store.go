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
	"go.etcd.io/bbolt"
	"time"
)

const (
	FieldEdgeRouterCertPEM     = "certPem"
	FieldEdgeRouterIsVerified  = "isVerified"
	FieldEdgeRouterHostname    = "hostname"
	FieldEdgeRouterProtocols   = "protocols"
	FieldEdgeRouterEnrollments = "enrollments"

	MethodEnrollEdgeRouterOtt = "erott"
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
	IsVerified          bool
	CertPem             *string
	Hostname            *string
	EdgeRouterProtocols map[string]string
	RoleAttributes      []string
	Enrollments         []string

	//old v4, migrations only
	EnrollmentToken     *string
	EnrollmentJwt       *string
	EnrollmentCreatedAt *time.Time
	EnrollmentExpiresAt *time.Time
}

func (entity *EdgeRouter) LoadValues(store boltz.CrudStore, bucket *boltz.TypedBucket) {
	_, err := store.GetParentStore().BaseLoadOneById(bucket.Tx(), entity.Id, &entity.Router)
	bucket.SetError(err)

	entity.CertPem = bucket.GetString(FieldEdgeRouterCertPEM)
	entity.IsVerified = bucket.GetBoolWithDefault(FieldEdgeRouterIsVerified, false)

	//old v4, migrations only
	entity.Enrollments = bucket.GetStringList(FieldEdgeRouterEnrollments)
	entity.Hostname = bucket.GetString(FieldEdgeRouterHostname)
	entity.EdgeRouterProtocols = toStringStringMap(bucket.GetMap(FieldEdgeRouterProtocols))
	entity.RoleAttributes = bucket.GetStringList(FieldRoleAttributes)
}

func (entity *EdgeRouter) SetValues(ctx *boltz.PersistContext) {
	entity.Router.SetValues(ctx.GetParentContext())

	store := ctx.Store.(*edgeRouterStoreImpl)
	ctx.SetStringP(FieldEdgeRouterCertPEM, entity.CertPem)
	ctx.SetBool(FieldEdgeRouterIsVerified, entity.IsVerified)
	ctx.SetStringP(FieldEdgeRouterHostname, entity.Hostname)
	ctx.SetMap(FieldEdgeRouterProtocols, toStringInterfaceMap(entity.EdgeRouterProtocols))
	ctx.SetStringList(FieldRoleAttributes, entity.RoleAttributes)

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
	store := &edgeRouterStoreImpl{
		baseStore: newChildBaseStore(stores, stores.Router),
	}
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

	store.symbolRoleAttributes = store.AddSetSymbol(FieldRoleAttributes, ast.NodeTypeString)

	store.indexName = store.GetParentStore().(db.RouterStore).GetNameIndex()
	store.indexRoleAttributes = store.AddSetIndex(store.symbolRoleAttributes)

	store.AddSymbol(FieldEdgeRouterIsVerified, ast.NodeTypeBool)
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

func (store *edgeRouterStoreImpl) DeleteById(ctx boltz.MutateContext, id string) error {
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

	if store.stores.Router.IsEntityPresent(ctx.Tx(), id) {
		return store.stores.Router.DeleteById(ctx, id)
	}

	return store.baseStore.DeleteById(ctx, id)
}

func (store *edgeRouterStoreImpl) GetRoleAttributesCursorProvider(values []string, semantic string) (ast.SetCursorProvider, error) {
	return store.getRoleAttributesCursorProvider(store.indexRoleAttributes, values, semantic)
}
