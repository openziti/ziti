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
	"math/rand"
	"sort"
)

type PolicyType int32

func (self PolicyType) String() string {
	if self == PolicyTypeDial {
		return PolicyTypeDialName
	}

	if self == PolicyTypeBind {
		return PolicyTypeBindName
	}

	return PolicyTypeInvalidName
}

const (
	FieldServicePolicyType = "type"

	PolicyTypeInvalidName = "Invalid"
	PolicyTypeDialName    = "Dial"
	PolicyTypeBindName    = "Bind"

	PolicyTypeInvalid PolicyType = 0
	PolicyTypeDial    PolicyType = 1
	PolicyTypeBind    PolicyType = 2
)

func newServicePolicy(name string) *ServicePolicy {
	policyType := PolicyTypeDial
	if rand.Int()%2 == 0 {
		policyType = PolicyTypeBind
	}
	return &ServicePolicy{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		Name:          name,
		PolicyType:    policyType,
		Semantic:      SemanticAllOf,
	}
}

type ServicePolicy struct {
	boltz.BaseExtEntity
	PolicyType        PolicyType
	Name              string
	Semantic          string
	IdentityRoles     []string
	ServiceRoles      []string
	PostureCheckRoles []string
}

func (entity *ServicePolicy) GetName() string {
	return entity.Name
}

func (entity *ServicePolicy) GetSemantic() string {
	return entity.Semantic
}

func (entity *ServicePolicy) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
	entity.PolicyType = PolicyType(bucket.GetInt32WithDefault(FieldServicePolicyType, int32(PolicyTypeDial)))
	entity.Semantic = bucket.GetStringWithDefault(FieldSemantic, SemanticAllOf)
	entity.IdentityRoles = bucket.GetStringList(FieldIdentityRoles)
	entity.ServiceRoles = bucket.GetStringList(FieldServiceRoles)
	entity.PostureCheckRoles = bucket.GetStringList(FieldPostureCheckRoles)
}

func (entity *ServicePolicy) SetValues(ctx *boltz.PersistContext) {
	if ctx.ProceedWithSet(FieldServicePolicyType) {
		if entity.PolicyType != PolicyTypeBind && entity.PolicyType != PolicyTypeDial {
			ctx.Bucket.SetError(errorz.NewFieldError("invalid policy type", FieldServicePolicyType, entity.PolicyType))
			return
		}
	} else {
		// PolicyType needs to be correct in the entity as we use it later
		// TODO: Add test for this
		entity.PolicyType = PolicyType(ctx.Bucket.GetInt32WithDefault(FieldServicePolicyType, int32(PolicyTypeDial)))
	}

	if err := validateRolesAndIds(FieldIdentityRoles, entity.IdentityRoles); err != nil {
		ctx.Bucket.SetError(err)
	}

	if err := validateRolesAndIds(FieldServiceRoles, entity.ServiceRoles); err != nil {
		ctx.Bucket.SetError(err)
	}

	if err := validateRolesAndIds(FieldPostureCheckRoles, entity.PostureCheckRoles); err != nil {
		ctx.Bucket.SetError(err)
	}

	if ctx.ProceedWithSet(FieldSemantic) && !isSemanticValid(entity.Semantic) {
		ctx.Bucket.SetError(errorz.NewFieldError("invalid semantic", FieldSemantic, entity.Semantic))
		return
	}

	entity.SetBaseValues(ctx)
	ctx.SetRequiredString(FieldName, entity.Name)
	ctx.SetInt32(FieldServicePolicyType, int32(entity.PolicyType))
	ctx.SetRequiredString(FieldSemantic, entity.Semantic)
	servicePolicyStore := ctx.Store.(*servicePolicyStoreImpl)

	sort.Strings(entity.ServiceRoles)
	sort.Strings(entity.IdentityRoles)
	sort.Strings(entity.PostureCheckRoles)

	oldIdentityRoles, valueSet := ctx.GetAndSetStringList(FieldIdentityRoles, entity.IdentityRoles)
	if valueSet && !stringz.EqualSlices(oldIdentityRoles, entity.IdentityRoles) {
		servicePolicyStore.identityRolesUpdated(ctx, entity)
	}

	oldServiceRoles, valueSet := ctx.GetAndSetStringList(FieldServiceRoles, entity.ServiceRoles)
	if valueSet && !stringz.EqualSlices(oldServiceRoles, entity.ServiceRoles) {
		servicePolicyStore.serviceRolesUpdated(ctx, entity)
	}

	oldPostureCheckRoles, valueSet := ctx.GetAndSetStringList(FieldPostureCheckRoles, entity.PostureCheckRoles)
	if valueSet && !stringz.EqualSlices(oldPostureCheckRoles, entity.PostureCheckRoles) {
		servicePolicyStore.postureCheckRolesUpdated(ctx, entity)
	}
}

func (entity *ServicePolicy) GetEntityType() string {
	return EntityTypeServicePolicies
}

func (entity *ServicePolicy) GetPolicyTypeName() string {
	return entity.PolicyType.String()
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

	indexName        boltz.ReadIndex
	symbolPolicyType boltz.EntitySymbol
	symbolSemantic   boltz.EntitySymbol

	symbolIdentityRoles     boltz.EntitySetSymbol
	symbolServiceRoles      boltz.EntitySetSymbol
	symbolPostureCheckRoles boltz.EntitySetSymbol

	symbolIdentities    boltz.EntitySetSymbol
	symbolServices      boltz.EntitySetSymbol
	symbolPostureChecks boltz.EntitySetSymbol

	identityCollection     boltz.LinkCollection
	serviceCollection      boltz.LinkCollection
	postureCheckCollection boltz.LinkCollection
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
	store.symbolPolicyType = store.AddSymbol(FieldServicePolicyType, ast.NodeTypeInt64)
	store.symbolSemantic = store.AddSymbol(FieldSemantic, ast.NodeTypeString)

	store.symbolIdentityRoles = store.AddPublicSetSymbol(FieldIdentityRoles, ast.NodeTypeString)
	store.symbolServiceRoles = store.AddPublicSetSymbol(FieldServiceRoles, ast.NodeTypeString)
	store.symbolPostureCheckRoles = store.AddPublicSetSymbol(FieldPostureCheckRoles, ast.NodeTypeString)

	store.symbolIdentities = store.AddFkSetSymbol(EntityTypeIdentities, store.stores.identity)
	store.symbolServices = store.AddFkSetSymbol(db.EntityTypeServices, store.stores.edgeService)
	store.symbolPostureChecks = store.AddFkSetSymbol(EntityTypePostureChecks, store.stores.postureCheck)
}

func (store *servicePolicyStoreImpl) initializeLinked() {
	store.serviceCollection = store.AddLinkCollection(store.symbolServices, store.stores.edgeService.symbolServicePolicies)
	store.identityCollection = store.AddLinkCollection(store.symbolIdentities, store.stores.identity.symbolServicePolicies)
	store.postureCheckCollection = store.AddLinkCollection(store.symbolPostureChecks, store.stores.postureCheck.symbolServicePolicies)
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
func (store *servicePolicyStoreImpl) serviceRolesUpdated(persistCtx *boltz.PersistContext, policy *ServicePolicy) {
	ctx := &roleAttributeChangeContext{
		tx:                    persistCtx.Bucket.Tx(),
		rolesSymbol:           store.symbolServiceRoles,
		linkCollection:        store.serviceCollection,
		relatedLinkCollection: store.identityCollection,
		ErrorHolder:           persistCtx.Bucket,
	}
	if policy.PolicyType == PolicyTypeDial {
		ctx.denormLinkCollection = store.stores.edgeService.dialIdentitiesCollection
		ctx.changeHandler = func(fromId, toId []byte, add bool) {
			ctx.addServicePolicyEvent(toId, fromId, PolicyTypeDial, add)
		}
	} else {
		ctx.denormLinkCollection = store.stores.edgeService.bindIdentitiesCollection
		ctx.changeHandler = func(fromId, toId []byte, add bool) {
			ctx.addServicePolicyEvent(toId, fromId, PolicyTypeBind, add)
		}
	}
	EvaluatePolicy(ctx, policy, store.stores.edgeService.symbolRoleAttributes)
}

func (store *servicePolicyStoreImpl) identityRolesUpdated(persistCtx *boltz.PersistContext, policy *ServicePolicy) {
	ctx := &roleAttributeChangeContext{
		tx:                    persistCtx.Bucket.Tx(),
		rolesSymbol:           store.symbolIdentityRoles,
		linkCollection:        store.identityCollection,
		relatedLinkCollection: store.serviceCollection,
		ErrorHolder:           persistCtx.Bucket,
	}

	if policy.PolicyType == PolicyTypeDial {
		ctx.denormLinkCollection = store.stores.identity.dialServicesCollection
		ctx.changeHandler = func(fromId, toId []byte, add bool) {
			ctx.addServicePolicyEvent(fromId, toId, PolicyTypeDial, add)
		}
	} else {
		ctx.denormLinkCollection = store.stores.identity.bindServicesCollection
		ctx.changeHandler = func(fromId, toId []byte, add bool) {
			ctx.addServicePolicyEvent(fromId, toId, PolicyTypeBind, add)
		}
	}

	EvaluatePolicy(ctx, policy, store.stores.identity.symbolRoleAttributes)
}

func (store *servicePolicyStoreImpl) postureCheckRolesUpdated(persistCtx *boltz.PersistContext, policy *ServicePolicy) {
	ctx := &roleAttributeChangeContext{
		tx:                    persistCtx.Bucket.Tx(),
		rolesSymbol:           store.symbolPostureCheckRoles,
		linkCollection:        store.postureCheckCollection,
		relatedLinkCollection: store.serviceCollection,
		ErrorHolder:           persistCtx.Bucket,
	}

	ctx.changeHandler = func(fromId, toId []byte, add bool) {
		ctx.addServiceUpdatedEvent(store.baseStore, ctx.tx, toId)
	}

	if policy.PolicyType == PolicyTypeDial {
		ctx.denormLinkCollection = store.stores.postureCheck.dialServicesCollection
	} else {
		ctx.denormLinkCollection = store.stores.postureCheck.bindServicesCollection
	}

	EvaluatePolicy(ctx, policy, store.stores.postureCheck.symbolRoleAttributes)
}

func (store *servicePolicyStoreImpl) DeleteById(ctx boltz.MutateContext, id string) error {
	policy, err := store.LoadOneById(ctx.Tx(), id)
	if err != nil {
		return err
	}
	policy.IdentityRoles = nil
	policy.ServiceRoles = nil
	policy.PostureCheckRoles = nil

	err = store.Update(ctx, policy, nil)
	if err != nil {
		return fmt.Errorf("failure while clearing policy before delete: %w", err)
	}
	return store.BaseStore.DeleteById(ctx, id)
}

func (store *servicePolicyStoreImpl) CheckIntegrity(tx *bbolt.Tx, fix bool, errorSink func(err error, fixed bool)) error {
	ctx := &denormCheckCtx{
		name:                   "service-policies/bind",
		tx:                     tx,
		sourceStore:            store.stores.identity,
		targetStore:            store.stores.edgeService,
		policyStore:            store,
		sourceCollection:       store.identityCollection,
		targetCollection:       store.serviceCollection,
		targetDenormCollection: store.stores.identity.bindServicesCollection,
		errorSink:              errorSink,
		repair:                 fix,
		policyFilter: func(policyId []byte) bool {
			policyType := PolicyTypeInvalid
			if result := boltz.FieldToInt32(store.symbolPolicyType.Eval(tx, policyId)); result != nil {
				policyType = PolicyType(*result)
			}
			return policyType == PolicyTypeBind
		},
	}
	if err := validatePolicyDenormalization(ctx); err != nil {
		return err
	}

	ctx = &denormCheckCtx{
		name:                   "service-policies/dial",
		tx:                     tx,
		sourceStore:            store.stores.identity,
		targetStore:            store.stores.edgeService,
		policyStore:            store,
		sourceCollection:       store.identityCollection,
		targetCollection:       store.serviceCollection,
		targetDenormCollection: store.stores.identity.dialServicesCollection,
		errorSink:              errorSink,
		repair:                 fix,
		policyFilter: func(policyId []byte) bool {
			policyType := PolicyTypeInvalid
			if result := boltz.FieldToInt32(store.symbolPolicyType.Eval(tx, policyId)); result != nil {
				policyType = PolicyType(*result)
			}
			return policyType == PolicyTypeDial
		},
	}

	if err := validatePolicyDenormalization(ctx); err != nil {
		return err
	}

	return store.BaseStore.CheckIntegrity(tx, fix, errorSink)
}
