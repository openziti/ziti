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

func newServiceEdgeRouterPolicy(name string) *ServiceEdgeRouterPolicy {
	return &ServiceEdgeRouterPolicy{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		Name:          name,
		Semantic:      SemanticAllOf,
	}
}

type ServiceEdgeRouterPolicy struct {
	boltz.BaseExtEntity
	Name            string   `json:"name"`
	Semantic        string   `json:"semantic"`
	ServiceRoles    []string `json:"serviceRoles"`
	EdgeRouterRoles []string `json:"edgeRouterRoles"`
}

func (entity *ServiceEdgeRouterPolicy) GetName() string {
	return entity.Name
}

func (entity *ServiceEdgeRouterPolicy) GetSemantic() string {
	return entity.Semantic
}

func (entity *ServiceEdgeRouterPolicy) GetEntityType() string {
	return EntityTypeServiceEdgeRouterPolicies
}

var _ ServiceEdgeRouterPolicyStore = (*serviceEdgeRouterPolicyStoreImpl)(nil)

type ServiceEdgeRouterPolicyStore interface {
	NameIndexed
	Store[*ServiceEdgeRouterPolicy]
}

func newServiceEdgeRouterPolicyStore(stores *stores) *serviceEdgeRouterPolicyStoreImpl {
	store := &serviceEdgeRouterPolicyStoreImpl{}
	store.baseStore = newBaseStore[*ServiceEdgeRouterPolicy](stores, store)
	store.InitImpl(store)
	return store
}

type serviceEdgeRouterPolicyStoreImpl struct {
	*baseStore[*ServiceEdgeRouterPolicy]

	indexName             boltz.ReadIndex
	symbolSemantic        boltz.EntitySymbol
	symbolServiceRoles    boltz.EntitySetSymbol
	symbolEdgeRouterRoles boltz.EntitySetSymbol
	symbolServices        boltz.EntitySetSymbol
	symbolEdgeRouters     boltz.EntitySetSymbol

	serviceCollection    boltz.LinkCollection
	edgeRouterCollection boltz.LinkCollection
}

func (store *serviceEdgeRouterPolicyStoreImpl) GetNameIndex() boltz.ReadIndex {
	return store.indexName
}

func (store *serviceEdgeRouterPolicyStoreImpl) NewEntity() *ServiceEdgeRouterPolicy {
	return &ServiceEdgeRouterPolicy{}
}

func (store *serviceEdgeRouterPolicyStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()

	store.indexName = store.addUniqueNameField()
	store.symbolSemantic = store.AddSymbol(FieldSemantic, ast.NodeTypeString)
	store.symbolServiceRoles = store.AddPublicSetSymbol(FieldServiceRoles, ast.NodeTypeString)
	store.symbolEdgeRouterRoles = store.AddPublicSetSymbol(FieldEdgeRouterRoles, ast.NodeTypeString)
	store.symbolServices = store.AddFkSetSymbol(EntityTypeServices, store.stores.edgeService)
	store.symbolEdgeRouters = store.AddFkSetSymbol(EntityTypeRouters, store.stores.edgeRouter)
}

func (store *serviceEdgeRouterPolicyStoreImpl) initializeLinked() {
	store.edgeRouterCollection = store.AddLinkCollection(store.symbolEdgeRouters, store.stores.edgeRouter.symbolServiceEdgeRouterPolicies)
	store.serviceCollection = store.AddLinkCollection(store.symbolServices, store.stores.edgeService.symbolServiceEdgeRouterPolicies)
}

func (store *serviceEdgeRouterPolicyStoreImpl) FillEntity(entity *ServiceEdgeRouterPolicy, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
	entity.Semantic = bucket.GetStringWithDefault(FieldSemantic, SemanticAllOf)
	entity.ServiceRoles = bucket.GetStringList(FieldServiceRoles)
	entity.EdgeRouterRoles = bucket.GetStringList(FieldEdgeRouterRoles)
}

func (store *serviceEdgeRouterPolicyStoreImpl) PersistEntity(entity *ServiceEdgeRouterPolicy, ctx *boltz.PersistContext) {
	if err := validateRolesAndIds(FieldServiceRoles, entity.ServiceRoles); err != nil {
		ctx.Bucket.SetError(err)
	}

	if err := validateRolesAndIds(FieldEdgeRouterRoles, entity.EdgeRouterRoles); err != nil {
		ctx.Bucket.SetError(err)
	}

	if ctx.ProceedWithSet(FieldSemantic) && !isSemanticValid(entity.Semantic) {
		ctx.Bucket.SetError(errorz.NewFieldError("invalid semantic", FieldSemantic, entity.Semantic))
		return
	}

	entity.SetBaseValues(ctx)
	ctx.SetRequiredString(FieldName, entity.Name)
	ctx.SetRequiredString(FieldSemantic, entity.Semantic)

	serviceEdgeRouterPolicyStore := ctx.Store.(*serviceEdgeRouterPolicyStoreImpl)

	sort.Strings(entity.EdgeRouterRoles)
	sort.Strings(entity.ServiceRoles)

	oldServiceRoles, valueSet := ctx.GetAndSetStringList(FieldServiceRoles, entity.ServiceRoles)
	if valueSet && !stringz.EqualSlices(oldServiceRoles, entity.ServiceRoles) {
		serviceEdgeRouterPolicyStore.serviceRolesUpdated(ctx, entity)
	}
	oldEdgeRouterRoles, valueSet := ctx.GetAndSetStringList(FieldEdgeRouterRoles, entity.EdgeRouterRoles)
	if valueSet && !stringz.EqualSlices(oldEdgeRouterRoles, entity.EdgeRouterRoles) {
		serviceEdgeRouterPolicyStore.edgeRouterRolesUpdated(ctx, entity)
	}
}

/*
Optimizations
 1. When changing policies if only ids have changed, only add/remove ids from groups as needed
 2. When related entities added/changed, only evaluate policies against that one entity (service/edge router/service),
    and just add/remove/ignore
 3. Related entity deletes should be handled automatically by FK Indexes on those entities (need to verify the reverse as well/deleting policy)
*/
func (store *serviceEdgeRouterPolicyStoreImpl) edgeRouterRolesUpdated(persistCtx *boltz.PersistContext, policy *ServiceEdgeRouterPolicy) {
	ctx := &roleAttributeChangeContext{
		tx:                    persistCtx.Bucket.Tx(),
		rolesSymbol:           store.symbolEdgeRouterRoles,
		linkCollection:        store.edgeRouterCollection,
		relatedLinkCollection: store.serviceCollection,
		denormLinkCollection:  store.stores.edgeRouter.servicesCollection,
		ErrorHolder:           persistCtx.Bucket,
	}
	EvaluatePolicy(ctx, policy, store.stores.edgeRouter.symbolRoleAttributes)
}

func (store *serviceEdgeRouterPolicyStoreImpl) serviceRolesUpdated(persistCtx *boltz.PersistContext, policy *ServiceEdgeRouterPolicy) {
	ctx := &roleAttributeChangeContext{
		tx:                    persistCtx.Bucket.Tx(),
		rolesSymbol:           store.symbolServiceRoles,
		linkCollection:        store.serviceCollection,
		relatedLinkCollection: store.edgeRouterCollection,
		denormLinkCollection:  store.stores.edgeService.edgeRoutersCollection,
		ErrorHolder:           persistCtx.Bucket,
	}
	EvaluatePolicy(ctx, policy, store.stores.edgeService.symbolRoleAttributes)
}

func (store *serviceEdgeRouterPolicyStoreImpl) DeleteById(ctx boltz.MutateContext, id string) error {
	policy, err := store.LoadOneById(ctx.Tx(), id)
	if err != nil {
		return err
	}
	policy.EdgeRouterRoles = nil
	policy.ServiceRoles = nil
	err = store.Update(ctx, policy, nil)
	if err != nil {
		return fmt.Errorf("failure while clearing policy before delete: %w", err)
	}
	return store.BaseStore.DeleteById(ctx, id)
}

func (store *serviceEdgeRouterPolicyStoreImpl) CheckIntegrity(mutateCtx boltz.MutateContext, fix bool, errorSink func(err error, fixed bool)) error {
	ctx := &denormCheckCtx{
		name:                   "service-edge-router-policies",
		mutateCtx:              mutateCtx,
		sourceStore:            store.stores.edgeService,
		targetStore:            store.stores.edgeRouter,
		policyStore:            store,
		sourceCollection:       store.serviceCollection,
		targetCollection:       store.edgeRouterCollection,
		targetDenormCollection: store.stores.edgeService.edgeRoutersCollection,
		errorSink:              errorSink,
		repair:                 fix,
	}
	if err := validatePolicyDenormalization(ctx); err != nil {
		return err
	}

	return store.BaseStore.CheckIntegrity(mutateCtx, fix, errorSink)
}
