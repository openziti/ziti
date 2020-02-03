package persistence

import (
	"fmt"
	"github.com/netfoundry/ziti-edge/controller/validation"
	"sort"

	"github.com/google/uuid"
	"github.com/netfoundry/ziti-foundation/storage/ast"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/netfoundry/ziti-foundation/util/stringz"
	"go.etcd.io/bbolt"
)

const (
	FieldServicePolicyType          = "type"
	FieldServicePolicyIdentityRoles = "identityRoles"
	FieldServicePolicyServiceRoles  = "serviceRoles"

	PolicyTypeDialName = "Dial"
	PolicyTypeBindName = "Bind"

	PolicyTypeInvalid int32 = iota
	PolicyTypeDial
	PolicyTypeBind
)

func newServicePolicy(name string) *ServicePolicy {
	return &ServicePolicy{
		BaseEdgeEntityImpl: BaseEdgeEntityImpl{Id: uuid.New().String()},
		Name:               name,
	}
}

type ServicePolicy struct {
	BaseEdgeEntityImpl
	PolicyType    int32
	Name          string
	Semantic      string
	IdentityRoles []string
	ServiceRoles  []string
}

func (entity *ServicePolicy) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
	entity.PolicyType = bucket.GetInt32WithDefault(FieldServicePolicyType, PolicyTypeInvalid)
	entity.Semantic = bucket.GetStringWithDefault(FieldSemantic, SemanticAllOf)
	entity.IdentityRoles = bucket.GetStringList(FieldServicePolicyIdentityRoles)
	entity.ServiceRoles = bucket.GetStringList(FieldServicePolicyServiceRoles)
}

func (entity *ServicePolicy) SetValues(ctx *boltz.PersistContext) {
	if entity.Semantic == "" {
		entity.Semantic = SemanticAllOf
	}

	if !stringz.Contains(validSemantics, entity.Semantic) {
		ctx.Bucket.SetError(validation.NewFieldError("invalid semantic", FieldSemantic, entity.Semantic))
		return
	}

	if len(entity.IdentityRoles) > 1 && stringz.Contains(entity.IdentityRoles, AllRole) {
		ctx.Bucket.SetError(validation.NewFieldError(fmt.Sprintf("if using %v, it should be the only role specified", AllRole),
			FieldServicePolicyIdentityRoles, entity.IdentityRoles))
		return
	}

	if len(entity.ServiceRoles) > 1 && stringz.Contains(entity.ServiceRoles, AllRole) {
		ctx.Bucket.SetError(validation.NewFieldError(fmt.Sprintf("if using %v, it should be the only role specified", AllRole),
			FieldServicePolicyServiceRoles, entity.ServiceRoles))
		return
	}

	entity.SetBaseValues(ctx)
	ctx.SetString(FieldName, entity.Name)
	ctx.SetInt32(FieldServicePolicyType, entity.PolicyType)
	ctx.SetString(FieldSemantic, entity.Semantic)
	servicePolicyStore := ctx.Store.(*servicePolicyStoreImpl)

	sort.Strings(entity.ServiceRoles)
	sort.Strings(entity.IdentityRoles)

	oldIdentityRoles, valueSet := ctx.GetAndSetStringList(FieldServicePolicyIdentityRoles, entity.IdentityRoles)
	if valueSet && !stringz.EqualSlices(oldIdentityRoles, entity.IdentityRoles) {
		servicePolicyStore.identityRolesUpdated(ctx, entity)
	}
	oldServiceRoles, valueSet := ctx.GetAndSetStringList(FieldServicePolicyServiceRoles, entity.ServiceRoles)
	if valueSet && !stringz.EqualSlices(oldServiceRoles, entity.ServiceRoles) {
		servicePolicyStore.serviceRolesUpdated(ctx, entity)
	}
}

func (entity *ServicePolicy) GetEntityType() string {
	return EntityTypeServicePolicies
}

type ServicePolicyStore interface {
	Store
	LoadOneById(tx *bbolt.Tx, id string) (*ServicePolicy, error)
	LoadOneByName(tx *bbolt.Tx, id string) (*ServicePolicy, error)
}

func newServicePolicyStore(stores *stores) *servicePolicyStoreImpl {
	store := &servicePolicyStoreImpl{
		baseStore: newBaseStore(stores, EntityTypeServicePolicies),
	}
	store.InitImpl(store)
	return store
}

type servicePolicyStoreImpl struct {
	*baseStore

	indexName           boltz.ReadIndex
	symbolSemantic      boltz.EntitySymbol
	symbolIdentityRoles boltz.EntitySetSymbol
	symbolServiceRoles  boltz.EntitySetSymbol
	symbolIdentities    boltz.EntitySetSymbol
	symbolServices      boltz.EntitySetSymbol

	identityCollection boltz.LinkCollection
	serviceCollection  boltz.LinkCollection
}

func (store *servicePolicyStoreImpl) NewStoreEntity() boltz.BaseEntity {
	return &ServicePolicy{}
}

func (store *servicePolicyStoreImpl) initializeLocal() {
	store.addBaseFields()

	store.indexName = store.addUniqueNameField()
	store.AddSymbol(FieldServicePolicyType, ast.NodeTypeInt64)
	store.symbolSemantic = store.AddSymbol(FieldSemantic, ast.NodeTypeString)
	store.symbolIdentityRoles = store.AddSetSymbol(FieldServicePolicyIdentityRoles, ast.NodeTypeString)
	store.symbolServiceRoles = store.AddSetSymbol(FieldServicePolicyServiceRoles, ast.NodeTypeString)
	store.symbolIdentities = store.AddFkSetSymbol(EntityTypeIdentities, store.stores.identity)
	store.symbolServices = store.AddFkSetSymbol(EntityTypeServices, store.stores.edgeService)
}

func (store *servicePolicyStoreImpl) initializeLinked() {
	store.serviceCollection = store.AddLinkCollection(store.symbolServices, store.stores.edgeService.symbolServicePolicies)
	store.identityCollection = store.AddLinkCollection(store.symbolIdentities, store.stores.identity.symbolServicePolicies)
}

func (store *servicePolicyStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*ServicePolicy, error) {
	entity := &ServicePolicy{}
	if err := store.baseLoadOneById(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *servicePolicyStoreImpl) LoadOneByName(tx *bbolt.Tx, name string) (*ServicePolicy, error) {
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
func (store *servicePolicyStoreImpl) serviceRolesUpdated(ctx *boltz.PersistContext, policy *ServicePolicy) {
	roleIds, err := store.getEntityIdsForRoleSet(ctx.Bucket.Tx(), "serviceRoles", policy.ServiceRoles, policy.Semantic, store.stores.edgeService.indexRoleAttributes, store.stores.edgeService)
	if !ctx.Bucket.SetError(err) {
		ctx.Bucket.SetError(store.serviceCollection.SetLinks(ctx.Bucket.Tx(), policy.Id, roleIds))
	}
}

func (store *servicePolicyStoreImpl) identityRolesUpdated(ctx *boltz.PersistContext, policy *ServicePolicy) {
	roleIds, err := store.getEntityIdsForRoleSet(ctx.Bucket.Tx(), "identityRoles", policy.IdentityRoles, policy.Semantic, store.stores.identity.indexRoleAttributes, store.stores.identity)
	if !ctx.Bucket.SetError(err) {
		ctx.Bucket.SetError(store.identityCollection.SetLinks(ctx.Bucket.Tx(), policy.Id, roleIds))
	}
}
