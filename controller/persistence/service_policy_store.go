package persistence

import (
	"github.com/google/uuid"
	"github.com/netfoundry/ziti-foundation/storage/ast"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/netfoundry/ziti-foundation/util/stringz"
	"github.com/netfoundry/ziti-foundation/validation"
	"go.etcd.io/bbolt"
	"sort"
)

const (
	FieldServicePolicyType = "type"

	PolicyTypeDialName = "Dial"
	PolicyTypeBindName = "Bind"

	PolicyTypeInvalid int32 = iota
	PolicyTypeDial
	PolicyTypeBind
)

func newServicePolicy(name string) *ServicePolicy {
	return &ServicePolicy{
		BaseExtEntity: boltz.BaseExtEntity{Id: uuid.New().String()},
		Name:          name,
	}
}

type ServicePolicy struct {
	boltz.BaseExtEntity
	PolicyType    int32
	Name          string
	Semantic      string
	IdentityRoles []string
	ServiceRoles  []string
}

func (entity *ServicePolicy) GetName() string {
	return entity.Name
}

func (entity *ServicePolicy) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
	entity.PolicyType = bucket.GetInt32WithDefault(FieldServicePolicyType, PolicyTypeInvalid)
	entity.Semantic = bucket.GetStringWithDefault(FieldSemantic, SemanticAllOf)
	entity.IdentityRoles = bucket.GetStringList(FieldIdentityRoles)
	entity.ServiceRoles = bucket.GetStringList(FieldServiceRoles)
}

func (entity *ServicePolicy) SetValues(ctx *boltz.PersistContext) {
	if entity.Semantic == "" {
		entity.Semantic = SemanticAllOf
	}

	if err := validateRolesAndIds(FieldIdentityRoles, entity.IdentityRoles); err != nil {
		ctx.Bucket.SetError(err)
	}

	if err := validateRolesAndIds(FieldServiceRoles, entity.ServiceRoles); err != nil {
		ctx.Bucket.SetError(err)
	}

	if !isSemanticValid(entity.Semantic) {
		ctx.Bucket.SetError(validation.NewFieldError("invalid semantic", FieldSemantic, entity.Semantic))
		return
	}

	entity.SetBaseValues(ctx)
	ctx.SetString(FieldName, entity.Name)
	ctx.SetInt32(FieldServicePolicyType, entity.PolicyType)
	ctx.SetString(FieldSemantic, entity.Semantic)
	servicePolicyStore := ctx.Store.(*servicePolicyStoreImpl)

	sort.Strings(entity.ServiceRoles)
	sort.Strings(entity.IdentityRoles)

	oldIdentityRoles, valueSet := ctx.GetAndSetStringList(FieldIdentityRoles, entity.IdentityRoles)
	if valueSet && !stringz.EqualSlices(oldIdentityRoles, entity.IdentityRoles) {
		servicePolicyStore.identityRolesUpdated(ctx, entity)
	}
	oldServiceRoles, valueSet := ctx.GetAndSetStringList(FieldServiceRoles, entity.ServiceRoles)
	if valueSet && !stringz.EqualSlices(oldServiceRoles, entity.ServiceRoles) {
		servicePolicyStore.serviceRolesUpdated(ctx, entity)
	}
}

func (entity *ServicePolicy) GetEntityType() string {
	return EntityTypeServicePolicies
}

func (entity *ServicePolicy) GetPolicyTypeName() string {
	if entity.PolicyType == PolicyTypeBind {
		return PolicyTypeBindName
	}
	return PolicyTypeDialName
}

type ServicePolicyStore interface {
	NameIndexedStore
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

func (store *servicePolicyStoreImpl) GetNameIndex() boltz.ReadIndex {
	return store.indexName
}

func (store *servicePolicyStoreImpl) NewStoreEntity() boltz.Entity {
	return &ServicePolicy{}
}

func (store *servicePolicyStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()

	store.indexName = store.addUniqueNameField()
	store.AddSymbol(FieldServicePolicyType, ast.NodeTypeInt64)
	store.symbolSemantic = store.AddSymbol(FieldSemantic, ast.NodeTypeString)
	store.symbolIdentityRoles = store.AddSetSymbol(FieldIdentityRoles, ast.NodeTypeString)
	store.symbolServiceRoles = store.AddSetSymbol(FieldServiceRoles, ast.NodeTypeString)
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
