package persistence

import (
	"fmt"
	"github.com/openziti/edge/eid"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/foundation/storage/ast"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/openziti/foundation/util/errorz"
	"github.com/openziti/foundation/util/stringz"
	"go.etcd.io/bbolt"
	"sort"
)

func newEdgeRouterPolicy(name string) *EdgeRouterPolicy {
	return &EdgeRouterPolicy{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		Name:          name,
	}
}

type EdgeRouterPolicy struct {
	boltz.BaseExtEntity
	Name            string
	Semantic        string
	IdentityRoles   []string
	EdgeRouterRoles []string
}

func (entity *EdgeRouterPolicy) GetName() string {
	return entity.Name
}

func (entity *EdgeRouterPolicy) GetSemantic() string {
	return entity.Semantic
}

func (entity *EdgeRouterPolicy) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
	entity.Semantic = bucket.GetStringWithDefault(FieldSemantic, SemanticAllOf)
	entity.IdentityRoles = bucket.GetStringList(FieldIdentityRoles)
	entity.EdgeRouterRoles = bucket.GetStringList(FieldEdgeRouterRoles)
}

func (entity *EdgeRouterPolicy) SetValues(ctx *boltz.PersistContext) {
	if entity.Semantic == "" {
		entity.Semantic = SemanticAllOf
	}

	if err := validateRolesAndIds(FieldIdentityRoles, entity.IdentityRoles); err != nil {
		ctx.Bucket.SetError(err)
	}

	if err := validateRolesAndIds(FieldEdgeRouterRoles, entity.EdgeRouterRoles); err != nil {
		ctx.Bucket.SetError(err)
	}

	if !isSemanticValid(entity.Semantic) {
		ctx.Bucket.SetError(errorz.NewFieldError("invalid semantic", FieldSemantic, entity.Semantic))
		return
	}

	entity.SetBaseValues(ctx)
	ctx.SetString(FieldName, entity.Name)
	ctx.SetString(FieldSemantic, entity.Semantic)

	edgeRouterPolicyStore := ctx.Store.(*edgeRouterPolicyStoreImpl)

	sort.Strings(entity.EdgeRouterRoles)
	sort.Strings(entity.IdentityRoles)

	oldIdentityRoles, valueSet := ctx.GetAndSetStringList(FieldIdentityRoles, entity.IdentityRoles)
	if valueSet && !stringz.EqualSlices(oldIdentityRoles, entity.IdentityRoles) {
		edgeRouterPolicyStore.identityRolesUpdated(ctx, entity)
	}
	oldEdgeRouterRoles, valueSet := ctx.GetAndSetStringList(FieldEdgeRouterRoles, entity.EdgeRouterRoles)
	if valueSet && !stringz.EqualSlices(oldEdgeRouterRoles, entity.EdgeRouterRoles) {
		edgeRouterPolicyStore.edgeRouterRolesUpdated(ctx, entity)
	}
}

func (entity *EdgeRouterPolicy) GetEntityType() string {
	return EntityTypeEdgeRouterPolicies
}

type EdgeRouterPolicyStore interface {
	NameIndexedStore
	LoadOneById(tx *bbolt.Tx, id string) (*EdgeRouterPolicy, error)
	LoadOneByName(tx *bbolt.Tx, id string) (*EdgeRouterPolicy, error)
}

func newEdgeRouterPolicyStore(stores *stores) *edgeRouterPolicyStoreImpl {
	store := &edgeRouterPolicyStoreImpl{
		baseStore: newBaseStore(stores, EntityTypeEdgeRouterPolicies),
	}
	store.InitImpl(store)
	return store
}

type edgeRouterPolicyStoreImpl struct {
	*baseStore

	indexName             boltz.ReadIndex
	symbolSemantic        boltz.EntitySymbol
	symbolIdentityRoles   boltz.EntitySetSymbol
	symbolEdgeRouterRoles boltz.EntitySetSymbol
	symbolIdentities      boltz.EntitySetSymbol
	symbolEdgeRouters     boltz.EntitySetSymbol

	identityCollection   boltz.LinkCollection
	edgeRouterCollection boltz.LinkCollection
}

func (store *edgeRouterPolicyStoreImpl) NewStoreEntity() boltz.Entity {
	return &EdgeRouterPolicy{}
}

func (store *edgeRouterPolicyStoreImpl) GetNameIndex() boltz.ReadIndex {
	return store.indexName
}

func (store *edgeRouterPolicyStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()

	store.indexName = store.addUniqueNameField()
	store.symbolSemantic = store.AddSymbol(FieldSemantic, ast.NodeTypeString)
	store.symbolIdentityRoles = store.AddPublicSetSymbol(FieldIdentityRoles, ast.NodeTypeString)
	store.symbolEdgeRouterRoles = store.AddPublicSetSymbol(FieldEdgeRouterRoles, ast.NodeTypeString)
	store.symbolIdentities = store.AddFkSetSymbol(EntityTypeIdentities, store.stores.identity)
	store.symbolEdgeRouters = store.AddFkSetSymbol(db.EntityTypeRouters, store.stores.edgeRouter)

	store.AddConstraint(boltz.NewSystemEntityEnforcementConstraint(store))
}

func (store *edgeRouterPolicyStoreImpl) initializeLinked() {
	store.edgeRouterCollection = store.AddLinkCollection(store.symbolEdgeRouters, store.stores.edgeRouter.symbolEdgeRouterPolicies)
	store.identityCollection = store.AddLinkCollection(store.symbolIdentities, store.stores.identity.symbolEdgeRouterPolicies)
}

func (store *edgeRouterPolicyStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*EdgeRouterPolicy, error) {
	entity := &EdgeRouterPolicy{}
	if err := store.baseLoadOneById(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *edgeRouterPolicyStoreImpl) LoadOneByName(tx *bbolt.Tx, name string) (*EdgeRouterPolicy, error) {
	id := store.indexName.Read(tx, []byte(name))
	if id != nil {
		return store.LoadOneById(tx, string(id))
	}
	return nil, nil
}

/*
Optimizations
1. When changing policies if only ids have changed, only add/remove ids from groups as needed
2. When related entities added/changed, only evaluate policies against that one entity (identity/edge router/service),
   and just add/remove/ignore
3. Related entity deletes should be handled automatically by FK Indexes on those entities (need to verify the reverse as well/deleting policy)
*/
func (store *edgeRouterPolicyStoreImpl) edgeRouterRolesUpdated(persistCtx *boltz.PersistContext, policy *EdgeRouterPolicy) {
	ctx := &roleAttributeChangeContext{
		tx:                    persistCtx.Bucket.Tx(),
		rolesSymbol:           store.symbolEdgeRouterRoles,
		linkCollection:        store.edgeRouterCollection,
		relatedLinkCollection: store.identityCollection,
		denormLinkCollection:  store.stores.edgeRouter.identitiesCollection,
		ErrorHolder:           persistCtx.Bucket,
	}
	EvaluatePolicy(ctx, policy, store.stores.edgeRouter.symbolRoleAttributes)
}

func (store *edgeRouterPolicyStoreImpl) identityRolesUpdated(persistCtx *boltz.PersistContext, policy *EdgeRouterPolicy) {
	ctx := &roleAttributeChangeContext{
		tx:                    persistCtx.Bucket.Tx(),
		rolesSymbol:           store.symbolIdentityRoles,
		linkCollection:        store.identityCollection,
		relatedLinkCollection: store.edgeRouterCollection,
		denormLinkCollection:  store.stores.identity.edgeRoutersCollection,
		ErrorHolder:           persistCtx.Bucket,
	}
	EvaluatePolicy(ctx, policy, store.stores.identity.symbolRoleAttributes)
}

func (store *edgeRouterPolicyStoreImpl) DeleteById(ctx boltz.MutateContext, id string) error {
	policy, err := store.LoadOneById(ctx.Tx(), id)
	if err != nil {
		return err
	}
	if !policy.IsSystem || ctx.IsSystemContext() {
		policy.EdgeRouterRoles = nil
		policy.IdentityRoles = nil
		err = store.Update(ctx, policy, nil)
		if err != nil {
			return fmt.Errorf("failure while clearing policy before delete: %w", err)
		}
	}
	return store.BaseStore.DeleteById(ctx, id)
}

func (store *edgeRouterPolicyStoreImpl) CheckIntegrity(tx *bbolt.Tx, fix bool, errorSink func(error, bool)) error {
	ctx := &denormCheckCtx{
		name:                   "edge-router-policies",
		tx:                     tx,
		sourceStore:            store.stores.identity,
		targetStore:            store.stores.edgeRouter,
		policyStore:            store,
		sourceCollection:       store.identityCollection,
		targetCollection:       store.edgeRouterCollection,
		targetDenormCollection: store.stores.identity.edgeRoutersCollection,
		repair:                 fix,
		errorSink:              errorSink,
	}
	if err := validatePolicyDenormalization(ctx); err != nil {
		return err
	}

	return store.BaseStore.CheckIntegrity(tx, fix, errorSink)
}
