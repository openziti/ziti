package db

import (
	"fmt"
	"sort"

	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/controller/storage/ast"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
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

func (self PolicyType) IsDial() bool {
	return self == PolicyTypeDial
}

func (self PolicyType) IsBind() bool {
	return self == PolicyTypeBind
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

// policyTypeSymbolMapper maps the stored int32 policy type to its "Dial"/"Bind"
// string form so that queries can filter by the same names used in the REST API.
type policyTypeSymbolMapper struct{}

func (policyTypeSymbolMapper) Map(_ boltz.EntitySymbol, fieldType boltz.FieldType, value []byte) (boltz.FieldType, []byte) {
	if fieldType != boltz.TypeInt32 {
		return fieldType, value
	}
	if intVal := boltz.BytesToInt32(value); intVal != nil {
		return boltz.TypeString, []byte(GetPolicyTypeForId(*intVal).String())
	}
	return fieldType, value
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

type ServicePolicyChangeEventListener func(event *edge_ctrl_pb.DataState_ServicePolicyChange)

var _ ServicePolicyStore = (*servicePolicyStoreImpl)(nil)

type ServicePolicyStore interface {
	NameIndexed
	Store[*ServicePolicy]
	GetIdentityRoleAttributesIndex() boltz.SetReadIndex
	GetServiceRoleAttributesIndex() boltz.SetReadIndex
	GetPostureCheckRoleAttributesIndex() boltz.SetReadIndex
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

	indexIdentityRoleAttributes     boltz.SetReadIndex
	indexServiceRoleAttributes      boltz.SetReadIndex
	indexPostureCheckRoleAttributes boltz.SetReadIndex

	symbolIdentities    boltz.EntitySetSymbol
	symbolServices      boltz.EntitySetSymbol
	symbolPostureChecks boltz.EntitySetSymbol

	identityCollection     boltz.LinkCollection
	serviceCollection      boltz.LinkCollection
	postureCheckCollection boltz.LinkCollection
}

// GetIdentityRoleAttributesIndex returns the derived set index of identity
// role-attribute references declared on service policies.
func (store *servicePolicyStoreImpl) GetIdentityRoleAttributesIndex() boltz.SetReadIndex {
	return store.indexIdentityRoleAttributes
}

// GetServiceRoleAttributesIndex returns the derived set index of service
// role-attribute references declared on service policies.
func (store *servicePolicyStoreImpl) GetServiceRoleAttributesIndex() boltz.SetReadIndex {
	return store.indexServiceRoleAttributes
}

// GetPostureCheckRoleAttributesIndex returns the derived set index of posture
// check role-attribute references declared on service policies.
func (store *servicePolicyStoreImpl) GetPostureCheckRoleAttributesIndex() boltz.SetReadIndex {
	return store.indexPostureCheckRoleAttributes
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
	store.symbolPolicyType = store.AddSymbol(FieldServicePolicyType, ast.NodeTypeString)
	store.MapSymbol(FieldServicePolicyType, policyTypeSymbolMapper{})
	store.symbolSemantic = store.AddSymbol(FieldSemantic, ast.NodeTypeString)

	store.symbolIdentityRoles = store.AddPublicSetSymbol(FieldIdentityRoles, ast.NodeTypeString)
	store.symbolServiceRoles = store.AddPublicSetSymbol(FieldServiceRoles, ast.NodeTypeString)
	store.symbolPostureCheckRoles = store.AddPublicSetSymbol(FieldPostureCheckRoles, ast.NodeTypeString)

	store.indexIdentityRoleAttributes = store.AddSetIndexWithTransform(store.symbolIdentityRoles, roleAttributeOnlyTransform)
	store.indexServiceRoleAttributes = store.AddSetIndexWithTransform(store.symbolServiceRoles, roleAttributeOnlyTransform)
	store.indexPostureCheckRoleAttributes = store.AddSetIndexWithTransform(store.symbolPostureCheckRoles, roleAttributeOnlyTransform)

	store.symbolIdentities = store.AddFkSetSymbol(EntityTypeIdentities, store.stores.identity)
	store.symbolServices = store.AddFkSetSymbol(EntityTypeServices, store.stores.service)
	store.symbolPostureChecks = store.AddFkSetSymbol(EntityTypePostureChecks, store.stores.postureCheck)

	store.MakeSymbolPublic(EntityTypeIdentities)
	store.MakeSymbolPublic(EntityTypeServices)
}

func (store *servicePolicyStoreImpl) initializeLinked() {
	store.serviceCollection = store.AddLinkCollection(store.symbolServices, store.stores.service.symbolServicePolicies)
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
	policyTypeChanged := false

	currentPolicyType := GetPolicyTypeForId(ctx.Bucket.GetInt32WithDefault(FieldServicePolicyType, PolicyTypeDial.Id()))
	if ctx.ProceedWithSet(FieldServicePolicyType) {
		if entity.PolicyType != PolicyTypeBind && entity.PolicyType != PolicyTypeDial {
			ctx.Bucket.SetError(errorz.NewFieldError("invalid policy type", FieldServicePolicyType, entity.PolicyType))
			return
		}
		policyTypeChanged = entity.PolicyType != currentPolicyType
	} else {
		// PolicyType needs to be correct in the entity as we use it later
		// TODO: Add test for this
		entity.PolicyType = currentPolicyType
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
	reevaluate := semanticChanged(ctx, entity.Semantic)

	entity.SetBaseValues(ctx)
	ctx.SetRequiredString(FieldName, entity.Name)
	ctx.SetInt32(FieldServicePolicyType, entity.PolicyType.Id())
	ctx.SetRequiredString(FieldSemantic, entity.Semantic)
	servicePolicyStore := ctx.Store.(*servicePolicyStoreImpl)

	sort.Strings(entity.ServiceRoles)
	sort.Strings(entity.IdentityRoles)
	sort.Strings(entity.PostureCheckRoles)

	if policyTypeChanged && !ctx.IsCreate {
		// if the policy type has changed, we need to remove all roles for the old policy type and then all the roles
		// for the new policy type

		updatedFields := ctx.FieldChecker
		if updatedFields == nil {
			updatedFields = FieldCheckerF(func(s string) bool {
				return true
			})
		}

		ctx.FieldChecker = FieldCheckerF(func(s string) bool {
			return s == FieldIdentityRoles || s == FieldServiceRoles || s == FieldPostureCheckRoles || updatedFields.IsUpdated(s)
		})

		newIdentityRoles := entity.IdentityRoles
		newServiceRoles := entity.ServiceRoles
		newPostureCheckRoles := entity.PostureCheckRoles

		entity.IdentityRoles = nil
		entity.ServiceRoles = nil
		entity.PostureCheckRoles = nil

		currentIdentityRoles, _ := ctx.GetAndSetStringList(FieldIdentityRoles, entity.IdentityRoles)
		currentServiceRoles, _ := ctx.GetAndSetStringList(FieldServiceRoles, entity.ServiceRoles)
		currentPostureCheckRoles, _ := ctx.GetAndSetStringList(FieldPostureCheckRoles, entity.PostureCheckRoles)

		if !updatedFields.IsUpdated(FieldIdentityRoles) {
			newIdentityRoles = currentIdentityRoles
		}

		if !updatedFields.IsUpdated(FieldServiceRoles) {
			newServiceRoles = currentServiceRoles
		}

		if !updatedFields.IsUpdated(FieldPostureCheckRoles) {
			newPostureCheckRoles = currentPostureCheckRoles
		}

		newPolicyType := entity.PolicyType
		entity.PolicyType = currentPolicyType

		servicePolicyStore.identityRolesUpdated(ctx, entity)
		servicePolicyStore.serviceRolesUpdated(ctx, entity)
		servicePolicyStore.postureCheckRolesUpdated(ctx, entity)

		entity.PolicyType = newPolicyType
		entity.IdentityRoles = newIdentityRoles
		entity.ServiceRoles = newServiceRoles
		entity.PostureCheckRoles = newPostureCheckRoles

		_, _ = ctx.GetAndSetStringList(FieldIdentityRoles, entity.IdentityRoles)
		_, _ = ctx.GetAndSetStringList(FieldServiceRoles, entity.ServiceRoles)

		servicePolicyStore.identityRolesUpdated(ctx, entity)
		servicePolicyStore.serviceRolesUpdated(ctx, entity)
		servicePolicyStore.postureCheckRolesUpdated(ctx, entity)
	} else {
		currentIdentityRoles, identityRolesSet := ctx.GetAndSetStringList(FieldIdentityRoles, entity.IdentityRoles)
		currentServiceRoles, serviceRolesSet := ctx.GetAndSetStringList(FieldServiceRoles, entity.ServiceRoles)
		currentPostureCheckRoles, postureCheckRolesSet := ctx.GetAndSetStringList(FieldPostureCheckRoles, entity.PostureCheckRoles)

		if reevaluate || (identityRolesSet && !stringz.EqualSlices(currentIdentityRoles, entity.IdentityRoles)) {
			servicePolicyStore.identityRolesUpdated(ctx, entity)
		}

		if reevaluate || (serviceRolesSet && !stringz.EqualSlices(currentServiceRoles, entity.ServiceRoles)) {
			servicePolicyStore.serviceRolesUpdated(ctx, entity)
		}
		if reevaluate || (postureCheckRolesSet && !stringz.EqualSlices(currentPostureCheckRoles, entity.PostureCheckRoles)) {
			servicePolicyStore.postureCheckRolesUpdated(ctx, entity)
		}
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
		mutateCtx:      persistCtx.MutateContext,
		rolesSymbol:    store.symbolServiceRoles,
		linkCollection: store.serviceCollection,
		// TEMPORARY(fabric-edge-collapse): keeps fabric-only services out of edge service policies
		// (both #all/role denorm and explicit @id); remove with the fabric/edge split.
		entityFilter: store.stores.service.isNotFabricOnly,
		ErrorHolder:  persistCtx.Bucket,
	}
	ctx.pairs = store.denormPairs(ctx, store.symbolServiceRoles, policy.PolicyType)
	ctx.changeHandler = func(policyId []byte, relatedId []byte, add bool) {
		ctx.notifyOfPolicyChangeEvent(policyId, relatedId, edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedService, add)
	}

	EvaluatePolicy(ctx, policy, store.stores.service.symbolRoleAttributes, store.stores.service.indexRoleAttributes)
}

func (store *servicePolicyStoreImpl) identityRolesUpdated(persistCtx *boltz.PersistContext, policy *ServicePolicy) {
	ctx := &roleAttributeChangeContext{
		mutateCtx:      persistCtx.MutateContext,
		rolesSymbol:    store.symbolIdentityRoles,
		linkCollection: store.identityCollection,
		ErrorHolder:    persistCtx.Bucket,
	}
	ctx.pairs = store.denormPairs(ctx, store.symbolIdentityRoles, policy.PolicyType)
	ctx.changeHandler = func(policyId []byte, relatedId []byte, add bool) {
		ctx.notifyOfPolicyChangeEvent(policyId, relatedId, edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedIdentity, add)
	}

	EvaluatePolicy(ctx, policy, store.stores.identity.symbolRoleAttributes, store.stores.identity.indexRoleAttributes)
}

func (store *servicePolicyStoreImpl) postureCheckRolesUpdated(persistCtx *boltz.PersistContext, policy *ServicePolicy) {
	ctx := &roleAttributeChangeContext{
		mutateCtx:      persistCtx.MutateContext,
		rolesSymbol:    store.symbolPostureCheckRoles,
		linkCollection: store.postureCheckCollection,
		ErrorHolder:    persistCtx.Bucket,
	}
	ctx.pairs = store.denormPairs(ctx, store.symbolPostureCheckRoles, policy.PolicyType)
	ctx.changeHandler = func(policyId []byte, relatedId []byte, add bool) {
		ctx.notifyOfPolicyChangeEvent(policyId, relatedId, edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedPostureCheck, add)
	}

	EvaluatePolicy(ctx, policy, store.stores.postureCheck.symbolRoleAttributes, store.stores.postureCheck.indexRoleAttributes)
}

// denormPairs returns the reference-counted tables fed by links between a policy of policyType and
// the entities selected by rolesSymbol, with the events each raises when a pair appears or
// disappears. Service links feed both the identity and the posture check tables; identity and
// posture check links feed the service tables of their own kind. Events are queued on ctx.
func (store *servicePolicyStoreImpl) denormPairs(ctx *roleAttributeChangeContext, rolesSymbol boltz.EntitySetSymbol, policyType PolicyType) []denormPair {
	stores := store.stores
	accessChanged := func(identityId, serviceId []byte, add bool) {
		ctx.addServicePolicyEvent(identityId, serviceId, policyType, add)
	}
	serviceChanged := func(serviceId []byte) {
		ctx.addServiceUpdatedEvent(stores, ctx.tx(), serviceId)
	}

	switch rolesSymbol {
	case store.symbolServiceRoles:
		identities := denormPair{related: store.identityCollection, counts: stores.service.bindIdentitiesCollection}
		postureChecks := denormPair{related: store.postureCheckCollection, counts: stores.service.bindPostureChecksCollection}
		if policyType == PolicyTypeDial {
			identities.counts = stores.service.dialIdentitiesCollection
			postureChecks.counts = stores.service.dialPostureChecksCollection
		}
		identities.onChange = func(serviceId, identityId []byte, add bool) {
			accessChanged(identityId, serviceId, add)
		}
		postureChecks.onChange = func(serviceId, _ []byte, _ bool) {
			serviceChanged(serviceId)
		}
		return []denormPair{identities, postureChecks}
	case store.symbolIdentityRoles:
		services := denormPair{related: store.serviceCollection, counts: stores.identity.bindServicesCollection, onChange: accessChanged}
		if policyType == PolicyTypeDial {
			services.counts = stores.identity.dialServicesCollection
		}
		return []denormPair{services}
	case store.symbolPostureCheckRoles:
		services := denormPair{related: store.serviceCollection, counts: stores.postureCheck.bindServicesCollection}
		if policyType == PolicyTypeDial {
			services.counts = stores.postureCheck.dialServicesCollection
		}
		services.onChange = func(_, serviceId []byte, _ bool) {
			serviceChanged(serviceId)
		}
		return []denormPair{services}
	}
	ctx.SetError(errors.Errorf("no denormalized tables are fed by %v", rolesSymbol.GetName()))
	return nil
}

func (store *servicePolicyStoreImpl) DeleteById(ctx boltz.MutateContext, id string) error {
	policy, err := store.LoadById(ctx.Tx(), id)
	if err != nil {
		return err
	}

	if len(policy.IdentityRoles) != 0 || len(policy.ServiceRoles) != 0 || len(policy.PostureCheckRoles) != 0 {
		policy.IdentityRoles = nil
		policy.ServiceRoles = nil
		policy.PostureCheckRoles = nil

		err = store.Update(ctx, policy, nil)
		if err != nil {
			return fmt.Errorf("failure while clearing policy before delete: %w", err)
		}
	}

	return store.BaseStore.DeleteById(ctx, id)
}

func (store *servicePolicyStoreImpl) CheckIntegrity(mutateCtx boltz.MutateContext, fix bool, errorSink func(err error, fixed bool)) error {
	checks := []struct {
		name       string
		policyType PolicyType
		source     boltz.Store
		collection boltz.LinkCollection
		counts     boltz.RefCountedLinkCollection
	}{
		{"service-policies/bind", PolicyTypeBind, store.stores.identity, store.identityCollection, store.stores.identity.bindServicesCollection},
		{"service-policies/dial", PolicyTypeDial, store.stores.identity, store.identityCollection, store.stores.identity.dialServicesCollection},
		{"service-policies/posture-checks/bind", PolicyTypeBind, store.stores.postureCheck, store.postureCheckCollection, store.stores.postureCheck.bindServicesCollection},
		{"service-policies/posture-checks/dial", PolicyTypeDial, store.stores.postureCheck, store.postureCheckCollection, store.stores.postureCheck.dialServicesCollection},
	}

	for _, check := range checks {
		policyType := check.policyType
		ctx := &denormCheckCtx{
			name:                   check.name,
			mutateCtx:              mutateCtx,
			sourceStore:            check.source,
			targetStore:            store.stores.service,
			policyStore:            store,
			sourceCollection:       check.collection,
			targetCollection:       store.serviceCollection,
			targetDenormCollection: check.counts,
			errorSink:              errorSink,
			repair:                 fix,
			policyFilter: func(policyId []byte) bool {
				return store.getPolicyType(mutateCtx.Tx(), policyId) == policyType
			},
		}
		if err := validatePolicyDenormalization(ctx); err != nil {
			return err
		}
	}

	return store.BaseStore.CheckIntegrity(mutateCtx, fix, errorSink)
}

// getPolicyType returns the stored type of the policy with the given id, or PolicyTypeInvalid when
// it has none.
func (store *servicePolicyStoreImpl) getPolicyType(tx *bbolt.Tx, policyId []byte) PolicyType {
	if result := boltz.FieldToInt32(store.symbolPolicyType.Eval(tx, policyId)); result != nil {
		return GetPolicyTypeForId(*result)
	}
	return PolicyTypeInvalid
}

type FieldCheckerF func(string) bool

func (f FieldCheckerF) IsUpdated(s string) bool {
	return f(s)
}
