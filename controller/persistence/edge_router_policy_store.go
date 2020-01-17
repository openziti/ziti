package persistence

import (
	"github.com/google/uuid"
	"github.com/netfoundry/ziti-foundation/storage/ast"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/netfoundry/ziti-foundation/util/stringz"
	"go.etcd.io/bbolt"
	"sort"
)

const (
	FieldEdgeRouterPolicyEdgeRouterRoles = "edgeRouterRoles"
	FieldEdgeRouterPolicyIdentityRoles   = "identityRoles"
)

func newEdgeRouterPolicy(name string) *EdgeRouterPolicy {
	return &EdgeRouterPolicy{
		BaseEdgeEntityImpl: BaseEdgeEntityImpl{Id: uuid.New().String()},
		Name:               name,
	}
}

type EdgeRouterPolicy struct {
	BaseEdgeEntityImpl
	Name            string
	IdentityRoles   []string
	EdgeRouterRoles []string
}

func (entity *EdgeRouterPolicy) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
	entity.IdentityRoles = bucket.GetStringList(FieldEdgeRouterPolicyIdentityRoles)
	entity.EdgeRouterRoles = bucket.GetStringList(FieldEdgeRouterPolicyEdgeRouterRoles)
}

func (entity *EdgeRouterPolicy) SetValues(ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldName, entity.Name)

	edgeRouterPolicyStore := ctx.Store.(*edgeRouterPolicyStoreImpl)

	sort.Strings(entity.EdgeRouterRoles)
	sort.Strings(entity.IdentityRoles)

	oldIdentityRoles := ctx.GetAndSetStringList(FieldEdgeRouterPolicyIdentityRoles, entity.IdentityRoles)
	if !stringz.EqualSlices(oldIdentityRoles, entity.IdentityRoles) {
		edgeRouterPolicyStore.identityRolesUpdated(ctx, entity)
	}
	oldEdgeRouterRoles := ctx.GetAndSetStringList(FieldEdgeRouterPolicyEdgeRouterRoles, entity.EdgeRouterRoles)
	if !stringz.EqualSlices(oldEdgeRouterRoles, entity.EdgeRouterRoles) {
		edgeRouterPolicyStore.edgeRouterRolesUpdated(ctx, entity)
	}
}

func (entity *EdgeRouterPolicy) GetEntityType() string {
	return EntityTypeEdgeRouterPolicies
}

type EdgeRouterPolicyStore interface {
	Store
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
	symbolIdentityRoles   boltz.EntitySetSymbol
	symbolEdgeRouterRoles boltz.EntitySetSymbol
	symbolIdentities      boltz.EntitySymbol
	symbolEdgeRouters     boltz.EntitySymbol

	identityCollection   boltz.LinkCollection
	edgeRouterCollection boltz.LinkCollection
}

func (store *edgeRouterPolicyStoreImpl) NewStoreEntity() boltz.BaseEntity {
	return &EdgeRouterPolicy{}
}

func (store *edgeRouterPolicyStoreImpl) initializeLocal() {
	store.addBaseFields()

	store.indexName = store.addUniqueNameField()
	store.symbolIdentityRoles = store.AddSetSymbol(FieldEdgeRouterPolicyIdentityRoles, ast.NodeTypeString)
	store.symbolEdgeRouterRoles = store.AddSetSymbol(FieldEdgeRouterPolicyEdgeRouterRoles, ast.NodeTypeString)
	store.symbolIdentities = store.AddFkSetSymbol(EntityTypeIdentities, store.stores.identity)
	store.symbolEdgeRouters = store.AddFkSetSymbol(EntityTypeEdgeRouters, store.stores.edgeService)
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
func (store *edgeRouterPolicyStoreImpl) edgeRouterRolesUpdated(ctx *boltz.PersistContext, policy *EdgeRouterPolicy) {
	roleIds, err := store.getEntityIdsForRoleSet(ctx.Bucket.Tx(), "edgeRouterRoles", policy.EdgeRouterRoles, store.stores.edgeRouter.indexRoleAttributes, store.stores.edgeRouter)
	if !ctx.Bucket.SetError(err) {
		ctx.Bucket.SetError(store.edgeRouterCollection.SetLinks(ctx.Bucket.Tx(), policy.Id, roleIds))
	}
}

func (store *edgeRouterPolicyStoreImpl) identityRolesUpdated(ctx *boltz.PersistContext, policy *EdgeRouterPolicy) {
	roleIds, err := store.getEntityIdsForRoleSet(ctx.Bucket.Tx(), "identityRoles", policy.IdentityRoles, store.stores.identity.indexRoleAttributes, store.stores.identity)
	if !ctx.Bucket.SetError(err) {
		ctx.Bucket.SetError(store.identityCollection.SetLinks(ctx.Bucket.Tx(), policy.Id, roleIds))
	}
}
