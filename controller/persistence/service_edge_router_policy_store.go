package persistence

import (
	"github.com/google/uuid"
	"github.com/netfoundry/ziti-edge/controller/validation"
	"github.com/netfoundry/ziti-foundation/storage/ast"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/netfoundry/ziti-foundation/util/stringz"
	"go.etcd.io/bbolt"
	"sort"
)

func newServiceEdgeRouterPolicy(name string) *ServiceEdgeRouterPolicy {
	return &ServiceEdgeRouterPolicy{
		BaseExtEntity: boltz.BaseExtEntity{Id: uuid.New().String()},
		Name:          name,
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

func (entity *ServiceEdgeRouterPolicy) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
	entity.Semantic = bucket.GetStringWithDefault(FieldSemantic, SemanticAllOf)
	entity.ServiceRoles = bucket.GetStringList(FieldServiceRoles)
	entity.EdgeRouterRoles = bucket.GetStringList(FieldEdgeRouterRoles)
}

func (entity *ServiceEdgeRouterPolicy) SetValues(ctx *boltz.PersistContext) {
	if entity.Semantic == "" {
		entity.Semantic = SemanticAllOf
	}

	if err := validateRolesAndIds(FieldServiceRoles, entity.ServiceRoles); err != nil {
		ctx.Bucket.SetError(err)
	}

	if err := validateRolesAndIds(FieldEdgeRouterRoles, entity.EdgeRouterRoles); err != nil {
		ctx.Bucket.SetError(err)
	}

	if !isSemanticValid(entity.Semantic) {
		ctx.Bucket.SetError(validation.NewFieldError("invalid semantic", FieldSemantic, entity.Semantic))
		return
	}

	entity.SetBaseValues(ctx)
	ctx.SetString(FieldName, entity.Name)
	ctx.SetString(FieldSemantic, entity.Semantic)

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
	store.symbolServiceRoles = store.AddSetSymbol(FieldServiceRoles, ast.NodeTypeString)
	store.symbolEdgeRouterRoles = store.AddSetSymbol(FieldEdgeRouterRoles, ast.NodeTypeString)
	store.symbolServices = store.AddFkSetSymbol(EntityTypeServices, store.stores.edgeService)
	store.symbolEdgeRouters = store.AddFkSetSymbol(EntityTypeEdgeRouters, store.stores.edgeRouter)
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
func (store *serviceEdgeRouterPolicyStoreImpl) edgeRouterRolesUpdated(ctx *boltz.PersistContext, policy *ServiceEdgeRouterPolicy) {
	roleIds, err := store.getEntityIdsForRoleSet(ctx.Bucket.Tx(), FieldEdgeRouterRoles, policy.EdgeRouterRoles, policy.Semantic, store.stores.edgeRouter.indexRoleAttributes, store.stores.edgeRouter)
	if !ctx.Bucket.SetError(err) {
		ctx.Bucket.SetError(store.edgeRouterCollection.SetLinks(ctx.Bucket.Tx(), policy.Id, roleIds))
	}
}

func (store *serviceEdgeRouterPolicyStoreImpl) serviceRolesUpdated(ctx *boltz.PersistContext, policy *ServiceEdgeRouterPolicy) {
	roleIds, err := store.getEntityIdsForRoleSet(ctx.Bucket.Tx(), FieldServiceRoles, policy.ServiceRoles, policy.Semantic, store.stores.edgeService.indexRoleAttributes, store.stores.edgeService)
	if !ctx.Bucket.SetError(err) {
		ctx.Bucket.SetError(store.serviceCollection.SetLinks(ctx.Bucket.Tx(), policy.Id, roleIds))
	}
}
