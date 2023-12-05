package db

import (
	"fmt"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	"sort"
)

type PolicyType string

func (self PolicyType) String() string {
	return string(self)
}

func (self PolicyType) Id() int32 {
	if self == PolicyTypeDial {
		return 1
	}
	if self == PolicyTypeBind {
		return 2
	}
	return 0
}

func GetPolicyTypeForId(policyTypeId int32) PolicyType {
	policyType := PolicyTypeInvalid
	if policyTypeId == PolicyTypeDial.Id() {
		policyType = PolicyTypeDial
	} else if policyTypeId == PolicyTypeBind.Id() {
		policyType = PolicyTypeBind
	}
	return policyType
}

const (
	FieldServicePolicyType = "type"

	PolicyTypeInvalidName = "Invalid"
	PolicyTypeDialName    = "Dial"
	PolicyTypeBindName    = "Bind"

	PolicyTypeInvalid PolicyType = PolicyTypeInvalidName
	PolicyTypeDial    PolicyType = PolicyTypeDialName
	PolicyTypeBind    PolicyType = PolicyTypeBindName
)

type ServicePolicy struct {
	boltz.BaseExtEntity
	PolicyType        PolicyType `json:"policyType"`
	Name              string     `json:"name"`
	Semantic          string     `json:"semantic"`
	IdentityRoles     []string   `json:"identityRoles"`
	ServiceRoles      []string   `json:"serviceRoles"`
	PostureCheckRoles []string   `json:"postureCheckRoles"`
}

func (entity *ServicePolicy) GetName() string {
	return entity.Name
}

func (entity *ServicePolicy) GetSemantic() string {
	return entity.Semantic
}

func (entity *ServicePolicy) GetEntityType() string {
	return EntityTypeServicePolicies
}

var _ ServicePolicyStore = (*servicePolicyStoreImpl)(nil)

type ServicePolicyStore interface {
	NameIndexed
	Store[*ServicePolicy]
}

func newServicePolicyStore(stores *stores) *servicePolicyStoreImpl {
	store := &servicePolicyStoreImpl{}
	store.baseStore = newBaseStore[*ServicePolicy](stores, store)
	store.InitImpl(store)
	return store
}

type servicePolicyStoreImpl struct {
	*baseStore[*ServicePolicy]

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

func (store *servicePolicyStoreImpl) NewEntity() *ServicePolicy {
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
	store.symbolServices = store.AddFkSetSymbol(EntityTypeServices, store.stores.edgeService)
	store.symbolPostureChecks = store.AddFkSetSymbol(EntityTypePostureChecks, store.stores.postureCheck)
}

func (store *servicePolicyStoreImpl) initializeLinked() {
	store.serviceCollection = store.AddLinkCollection(store.symbolServices, store.stores.edgeService.symbolServicePolicies)
	store.identityCollection = store.AddLinkCollection(store.symbolIdentities, store.stores.identity.symbolServicePolicies)
	store.postureCheckCollection = store.AddLinkCollection(store.symbolPostureChecks, store.stores.postureCheck.symbolServicePolicies)
}

func (store *servicePolicyStoreImpl) FillEntity(entity *ServicePolicy, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
	entity.PolicyType = GetPolicyTypeForId(bucket.GetInt32WithDefault(FieldServicePolicyType, PolicyTypeDial.Id()))
	entity.Semantic = bucket.GetStringWithDefault(FieldSemantic, SemanticAllOf)
	entity.IdentityRoles = bucket.GetStringList(FieldIdentityRoles)
	entity.ServiceRoles = bucket.GetStringList(FieldServiceRoles)
	entity.PostureCheckRoles = bucket.GetStringList(FieldPostureCheckRoles)
}

func (store *servicePolicyStoreImpl) PersistEntity(entity *ServicePolicy, ctx *boltz.PersistContext) {
	if ctx.ProceedWithSet(FieldServicePolicyType) {
		if entity.PolicyType != PolicyTypeBind && entity.PolicyType != PolicyTypeDial {
			ctx.Bucket.SetError(errorz.NewFieldError("invalid policy type", FieldServicePolicyType, entity.PolicyType))
			return
		}
	} else {
		// PolicyType needs to be correct in the entity as we use it later
		// TODO: Add test for this
		entity.PolicyType = GetPolicyTypeForId(ctx.Bucket.GetInt32WithDefault(FieldServicePolicyType, PolicyTypeDial.Id()))
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
	ctx.SetInt32(FieldServicePolicyType, entity.PolicyType.Id())
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
		ctx.addServiceUpdatedEvent(store.stores, ctx.tx, toId)
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

func (store *servicePolicyStoreImpl) CheckIntegrity(mutateCtx boltz.MutateContext, fix bool, errorSink func(err error, fixed bool)) error {
	ctx := &denormCheckCtx{
		name:                   "service-policies/bind",
		mutateCtx:              mutateCtx,
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
			if result := boltz.FieldToInt32(store.symbolPolicyType.Eval(mutateCtx.Tx(), policyId)); result != nil {
				policyType = GetPolicyTypeForId(*result)
			}
			return policyType == PolicyTypeBind
		},
	}
	if err := validatePolicyDenormalization(ctx); err != nil {
		return err
	}

	ctx = &denormCheckCtx{
		name:                   "service-policies/dial",
		mutateCtx:              mutateCtx,
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
			if result := boltz.FieldToInt32(store.symbolPolicyType.Eval(mutateCtx.Tx(), policyId)); result != nil {
				policyType = GetPolicyTypeForId(*result)
			}
			return policyType == PolicyTypeDial
		},
	}

	if err := validatePolicyDenormalization(ctx); err != nil {
		return err
	}

	return store.BaseStore.CheckIntegrity(mutateCtx, fix, errorSink)
}
