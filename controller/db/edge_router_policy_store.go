package db

import (
	"fmt"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/common/eid"
	"sort"
)

func newEdgeRouterPolicy(name string) *EdgeRouterPolicy {
	return &EdgeRouterPolicy{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		Name:          name,
		Semantic:      SemanticAllOf,
	}
}

type EdgeRouterPolicy struct {
	boltz.BaseExtEntity
	Name            string   `json:"name"`
	Semantic        string   `json:"semantic"`
	IdentityRoles   []string `json:"identityRoles"`
	EdgeRouterRoles []string `json:"edgeRouterRoles"`
}

func (entity *EdgeRouterPolicy) GetName() string {
	return entity.Name
}

func (entity *EdgeRouterPolicy) GetSemantic() string {
	return entity.Semantic
}

func (entity *EdgeRouterPolicy) GetEntityType() string {
	return EntityTypeEdgeRouterPolicies
}

var _ EdgeRouterPolicyStore = (*edgeRouterPolicyStoreImpl)(nil)

type EdgeRouterPolicyStore interface {
	NameIndexed
	Store[*EdgeRouterPolicy]
}

func newEdgeRouterPolicyStore(stores *stores) *edgeRouterPolicyStoreImpl {
	store := &edgeRouterPolicyStoreImpl{}
	store.baseStore = newBaseStore[*EdgeRouterPolicy](stores, store)
	store.InitImpl(store)
	return store
}

type edgeRouterPolicyStoreImpl struct {
	*baseStore[*EdgeRouterPolicy]

	indexName             boltz.ReadIndex
	symbolSemantic        boltz.EntitySymbol
	symbolIdentityRoles   boltz.EntitySetSymbol
	symbolEdgeRouterRoles boltz.EntitySetSymbol
	symbolIdentities      boltz.EntitySetSymbol
	symbolEdgeRouters     boltz.EntitySetSymbol

	identityCollection   boltz.LinkCollection
	edgeRouterCollection boltz.LinkCollection
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
	store.symbolEdgeRouters = store.AddFkSetSymbol(EntityTypeRouters, store.stores.edgeRouter)

	store.AddConstraint(boltz.NewSystemEntityEnforcementConstraint(store))
}

func (store *edgeRouterPolicyStoreImpl) initializeLinked() {
	store.edgeRouterCollection = store.AddLinkCollection(store.symbolEdgeRouters, store.stores.edgeRouter.symbolEdgeRouterPolicies)
	store.identityCollection = store.AddLinkCollection(store.symbolIdentities, store.stores.identity.symbolEdgeRouterPolicies)
}

func (store *edgeRouterPolicyStoreImpl) NewEntity() *EdgeRouterPolicy {
	return &EdgeRouterPolicy{}
}

func (store *edgeRouterPolicyStoreImpl) FillEntity(entity *EdgeRouterPolicy, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
	entity.Semantic = bucket.GetStringWithDefault(FieldSemantic, SemanticAllOf)
	entity.IdentityRoles = bucket.GetStringList(FieldIdentityRoles)
	entity.EdgeRouterRoles = bucket.GetStringList(FieldEdgeRouterRoles)
}

func (store *edgeRouterPolicyStoreImpl) PersistEntity(entity *EdgeRouterPolicy, ctx *boltz.PersistContext) {
	if err := validateRolesAndIds(FieldIdentityRoles, entity.IdentityRoles); err != nil {
		ctx.Bucket.SetError(err)
	}

	if err := validateRolesAndIds(FieldEdgeRouterRoles, entity.EdgeRouterRoles); err != nil {
		ctx.Bucket.SetError(err)
	}

	entity.SetBaseValues(ctx)
	ctx.SetRequiredString(FieldName, entity.Name)
	if ctx.ProceedWithSet(FieldSemantic) {
		if !isSemanticValid(entity.Semantic) {
			ctx.Bucket.SetError(errorz.NewFieldError("invalid semantic", FieldSemantic, entity.Semantic))
			return
		}
		ctx.SetRequiredString(FieldSemantic, entity.Semantic)
	}

	sort.Strings(entity.EdgeRouterRoles)
	sort.Strings(entity.IdentityRoles)

	oldIdentityRoles, valueSet := ctx.GetAndSetStringList(FieldIdentityRoles, entity.IdentityRoles)
	if valueSet && !stringz.EqualSlices(oldIdentityRoles, entity.IdentityRoles) {
		store.identityRolesUpdated(ctx, entity)
	}
	oldEdgeRouterRoles, valueSet := ctx.GetAndSetStringList(FieldEdgeRouterRoles, entity.EdgeRouterRoles)
	if valueSet && !stringz.EqualSlices(oldEdgeRouterRoles, entity.EdgeRouterRoles) {
		store.edgeRouterRolesUpdated(ctx, entity)
	}
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

func (store *edgeRouterPolicyStoreImpl) CheckIntegrity(ctx boltz.MutateContext, fix bool, errorSink func(error, bool)) error {
	checkCtx := &denormCheckCtx{
		name:                   "edge-router-policies",
		mutateCtx:              ctx,
		sourceStore:            store.stores.identity,
		targetStore:            store.stores.edgeRouter,
		policyStore:            store,
		sourceCollection:       store.identityCollection,
		targetCollection:       store.edgeRouterCollection,
		targetDenormCollection: store.stores.identity.edgeRoutersCollection,
		repair:                 fix,
		errorSink:              errorSink,
	}
	if err := validatePolicyDenormalization(checkCtx); err != nil {
		return err
	}

	return store.BaseStore.CheckIntegrity(ctx, fix, errorSink)
}
