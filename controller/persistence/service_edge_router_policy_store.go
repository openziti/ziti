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

func newServiceEdgeRouterPolicy(name string) *ServiceEdgeRouterPolicy {
	return &ServiceEdgeRouterPolicy{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		Name:          name,
		Semantic:      SemanticAllOf,
	}
}

type ServiceEdgeRouterPolicy struct {
	boltz.BaseExtEntity
	Name            string
	Semantic        string
	ServiceRoles    []string
	EdgeRouterRoles []string
}

func (entity *ServiceEdgeRouterPolicy) GetName() string {
	return entity.Name
}

func (entity *ServiceEdgeRouterPolicy) GetSemantic() string {
	return entity.Semantic
}

func (entity *ServiceEdgeRouterPolicy) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
	entity.Semantic = bucket.GetStringWithDefault(FieldSemantic, SemanticAllOf)
	entity.ServiceRoles = bucket.GetStringList(FieldServiceRoles)
	entity.EdgeRouterRoles = bucket.GetStringList(FieldEdgeRouterRoles)
}

func (entity *ServiceEdgeRouterPolicy) SetValues(ctx *boltz.PersistContext) {
	if err := validateRolesAndIds(FieldServiceRoles, entity.ServiceRoles); err != nil {
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
	ctx.SetRequiredString(FieldName, entity.Name)
	if ctx.ProceedWithSet(FieldSemantic) {
		if !isSemanticValid(entity.Semantic) {
			ctx.Bucket.SetError(errorz.NewFieldError("invalid semantic", FieldSemantic, entity.Semantic))
			return
		}
		ctx.SetRequiredString(FieldSemantic, entity.Semantic)
	}

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

func (entity *ServiceEdgeRouterPolicy) GetEntityType() string {
	return EntityTypeServiceEdgeRouterPolicies
}

type ServiceEdgeRouterPolicyStore interface {
	NameIndexedStore
	LoadOneById(tx *bbolt.Tx, id string) (*ServiceEdgeRouterPolicy, error)
	LoadOneByName(tx *bbolt.Tx, id string) (*ServiceEdgeRouterPolicy, error)
}

func newServiceEdgeRouterPolicyStore(stores *stores) *serviceEdgeRouterPolicyStoreImpl {
	store := &serviceEdgeRouterPolicyStoreImpl{
		baseStore: newBaseStore(stores, EntityTypeServiceEdgeRouterPolicies),
	}
	store.InitImpl(store)
	return store
}

type serviceEdgeRouterPolicyStoreImpl struct {
	*baseStore

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

func (store *serviceEdgeRouterPolicyStoreImpl) NewStoreEntity() boltz.Entity {
	return &ServiceEdgeRouterPolicy{}
}

func (store *serviceEdgeRouterPolicyStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()

	store.indexName = store.addUniqueNameField()
	store.symbolSemantic = store.AddSymbol(FieldSemantic, ast.NodeTypeString)
	store.symbolServiceRoles = store.AddPublicSetSymbol(FieldServiceRoles, ast.NodeTypeString)
	store.symbolEdgeRouterRoles = store.AddPublicSetSymbol(FieldEdgeRouterRoles, ast.NodeTypeString)
	store.symbolServices = store.AddFkSetSymbol(db.EntityTypeServices, store.stores.edgeService)
	store.symbolEdgeRouters = store.AddFkSetSymbol(db.EntityTypeRouters, store.stores.edgeRouter)
}

func (store *serviceEdgeRouterPolicyStoreImpl) initializeLinked() {
	store.edgeRouterCollection = store.AddLinkCollection(store.symbolEdgeRouters, store.stores.edgeRouter.symbolServiceEdgeRouterPolicies)
	store.serviceCollection = store.AddLinkCollection(store.symbolServices, store.stores.edgeService.symbolServiceEdgeRouterPolicies)
}

func (store *serviceEdgeRouterPolicyStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*ServiceEdgeRouterPolicy, error) {
	entity := &ServiceEdgeRouterPolicy{}
	if err := store.baseLoadOneById(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *serviceEdgeRouterPolicyStoreImpl) LoadOneByName(tx *bbolt.Tx, name string) (*ServiceEdgeRouterPolicy, error) {
	id := store.indexName.Read(tx, []byte(name))
	if id != nil {
		return store.LoadOneById(tx, string(id))
	}
	return nil, nil
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

func (store *serviceEdgeRouterPolicyStoreImpl) CheckIntegrity(tx *bbolt.Tx, fix bool, errorSink func(err error, fixed bool)) error {
	ctx := &denormCheckCtx{
		name:                   "service-edge-router-policies",
		tx:                     tx,
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

	return store.BaseStore.CheckIntegrity(tx, fix, errorSink)
}
