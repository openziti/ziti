/*
	Copyright 2019 Netfoundry, Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package persistence

import (
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/util"
	"github.com/netfoundry/ziti-foundation/storage/ast"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/netfoundry/ziti-foundation/util/errorz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

const (
	FieldIdentityType           = "type"
	FieldIdentityApiSessions    = "apiSessions"
	FieldIdentityIsDefaultAdmin = "isDefaultAdmin"
	FieldIdentityIsAdmin        = "isAdmin"
	FieldIdentityEnrollments    = "enrollments"
	FieldIdentityAuthenticators = "authenticators"
	FieldIdentityServiceConfigs = "serviceConfigs"
)

func NewIdentity(name string, identityTypeId string, roleAttributes ...string) *Identity {
	return &Identity{
		BaseEdgeEntityImpl: BaseEdgeEntityImpl{Id: uuid.New().String()},
		Name:               name,
		IdentityTypeId:     identityTypeId,
		RoleAttributes:     roleAttributes,
	}
}

type Identity struct {
	BaseEdgeEntityImpl
	Name           string
	IdentityTypeId string
	IsDefaultAdmin bool
	IsAdmin        bool
	Enrollments    []string
	Authenticators []string
	RoleAttributes []string
}

type ServiceConfig struct {
	ServiceId string
	ConfigId  string
}

var identityFieldMappings = map[string]string{FieldIdentityType: "identityTypeId"}

func (entity *Identity) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
	entity.IdentityTypeId = bucket.GetStringWithDefault(FieldIdentityType, "")
	entity.IsDefaultAdmin = bucket.GetBoolWithDefault(FieldIdentityIsDefaultAdmin, false)
	entity.IsAdmin = bucket.GetBoolWithDefault(FieldIdentityIsAdmin, false)
	entity.Authenticators = bucket.GetStringList(FieldIdentityAuthenticators)
	entity.Enrollments = bucket.GetStringList(FieldIdentityEnrollments)
	entity.RoleAttributes = bucket.GetStringList(FieldRoleAttributes)
}

func (entity *Identity) SetValues(ctx *boltz.PersistContext) {
	ctx.WithFieldOverrides(identityFieldMappings)

	entity.SetBaseValues(ctx)

	store := ctx.Store.(*identityStoreImpl)
	if ctx.IsCreate {
		ctx.SetString(FieldName, entity.Name)
	} else if oldValue, changed := ctx.GetAndSetString(FieldName, entity.Name); changed {
		store.nameChanged(ctx.Bucket, entity, *oldValue)
	}
	ctx.SetBool(FieldIdentityIsDefaultAdmin, entity.IsDefaultAdmin)
	ctx.SetBool(FieldIdentityIsAdmin, entity.IsAdmin)
	ctx.SetString(FieldIdentityType, entity.IdentityTypeId)
	ctx.SetLinkedIds(FieldIdentityEnrollments, entity.Enrollments)
	ctx.SetLinkedIds(FieldIdentityAuthenticators, entity.Authenticators)
	ctx.SetStringList(FieldRoleAttributes, entity.RoleAttributes)

	// index change won't fire if we don't have any roles on create, but we need to evaluate if we match any #all roles
	if ctx.IsCreate && len(entity.RoleAttributes) == 0 {
		store.rolesChanged(ctx.Bucket.Tx(), []byte(entity.Id), nil, nil, ctx.Bucket)
	}
}

func (entity *Identity) GetEntityType() string {
	return EntityTypeIdentities
}

func (entity *Identity) GetName() string {
	return entity.Name
}

type IdentityStore interface {
	Store
	LoadOneById(tx *bbolt.Tx, id string) (*Identity, error)
	LoadOneByName(tx *bbolt.Tx, id string) (*Identity, error)
	LoadOneByQuery(tx *bbolt.Tx, query string) (*Identity, error)
	AssignServiceConfigs(tx *bbolt.Tx, identityId string, serviceConfigs ...ServiceConfig) error
	RemoveServiceConfigs(tx *bbolt.Tx, identityId string, serviceConfigs ...ServiceConfig) error
	GetServiceConfigs(tx *bbolt.Tx, identityId string) ([]ServiceConfig, error)
	LoadServiceConfigsByServiceAndType(tx *bbolt.Tx, identityId string, configTypes map[string]struct{}) map[string]map[string]map[string]interface{}
}

func newIdentityStore(stores *stores) *identityStoreImpl {
	store := &identityStoreImpl{
		baseStore: newBaseStore(stores, EntityTypeIdentities),
	}
	store.InitImpl(store)
	return store
}

type identityStoreImpl struct {
	*baseStore

	indexName           boltz.ReadIndex
	indexRoleAttributes boltz.SetReadIndex

	symbolApiSessions        boltz.EntitySetSymbol
	symbolAuthenticators     boltz.EntitySetSymbol
	symbolEdgeRouterPolicies boltz.EntitySetSymbol
	symbolEnrollments        boltz.EntitySetSymbol
	symbolServicePolicies    boltz.EntitySetSymbol
	symbolIdentityTypeId     boltz.EntitySymbol
}

func (store *identityStoreImpl) NewStoreEntity() boltz.BaseEntity {
	return &Identity{}
}

func (store *identityStoreImpl) initializeLocal() {
	store.addBaseFields()
	store.indexRoleAttributes = store.addRoleAttributesField()

	store.indexName = store.addUniqueNameField()
	store.symbolApiSessions = store.AddFkSetSymbol(FieldIdentityApiSessions, store.stores.apiSession)
	store.symbolEdgeRouterPolicies = store.AddFkSetSymbol(EntityTypeEdgeRouterPolicies, store.stores.edgeRouterPolicy)
	store.symbolServicePolicies = store.AddFkSetSymbol(EntityTypeServicePolicies, store.stores.servicePolicy)
	store.symbolEnrollments = store.AddFkSetSymbol(FieldIdentityEnrollments, store.stores.enrollment)
	store.symbolAuthenticators = store.AddFkSetSymbol(FieldIdentityAuthenticators, store.stores.authenticator)

	store.symbolIdentityTypeId = store.AddFkSymbol(FieldIdentityType, store.stores.identityType)

	store.AddSymbol(FieldIdentityIsAdmin, ast.NodeTypeBool)
	store.AddSymbol(FieldIdentityIsDefaultAdmin, ast.NodeTypeBool)

	store.indexRoleAttributes.AddListener(store.rolesChanged)
}

func (store *identityStoreImpl) rolesChanged(tx *bbolt.Tx, rowId []byte, _ []boltz.FieldTypeAndValue, new []boltz.FieldTypeAndValue, holder errorz.ErrorHolder) {
	rolesSymbol := store.stores.edgeRouterPolicy.symbolIdentityRoles
	linkCollection := store.stores.edgeRouterPolicy.identityCollection
	semanticSymbol := store.stores.edgeRouterPolicy.symbolSemantic
	UpdateRelatedRoles(store, tx, string(rowId), rolesSymbol, linkCollection, new, holder, semanticSymbol)

	rolesSymbol = store.stores.servicePolicy.symbolIdentityRoles
	linkCollection = store.stores.servicePolicy.identityCollection
	semanticSymbol = store.stores.servicePolicy.symbolSemantic
	UpdateRelatedRoles(store, tx, string(rowId), rolesSymbol, linkCollection, new, holder, semanticSymbol)
}

func (store *identityStoreImpl) nameChanged(bucket *boltz.TypedBucket, entity NamedEdgeEntity, oldName string) {
	store.updateEntityNameReferences(bucket, store.stores.servicePolicy.symbolIdentityRoles, entity, oldName)
	store.updateEntityNameReferences(bucket, store.stores.edgeRouterPolicy.symbolIdentityRoles, entity, oldName)
}

func (store *identityStoreImpl) initializeLinked() {
	store.AddLinkCollection(store.symbolAuthenticators, store.stores.authenticator.symbolIdentityId)
	store.AddLinkCollection(store.symbolEnrollments, store.stores.enrollment.symbolIdentityId)
	store.AddLinkCollection(store.symbolEdgeRouterPolicies, store.stores.edgeRouterPolicy.symbolIdentities)
	store.AddLinkCollection(store.symbolServicePolicies, store.stores.servicePolicy.symbolIdentities)
}

func (store *identityStoreImpl) GetNameIndex() boltz.ReadIndex {
	return store.indexName
}

func (store *identityStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*Identity, error) {
	entity := &Identity{}
	if err := store.baseLoadOneById(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *identityStoreImpl) LoadOneByName(tx *bbolt.Tx, name string) (*Identity, error) {
	id := store.indexName.Read(tx, []byte(name))
	if id != nil {
		return store.LoadOneById(tx, string(id))
	}
	return nil, nil
}

func (store *identityStoreImpl) LoadOneByQuery(tx *bbolt.Tx, query string) (*Identity, error) {
	entity := &Identity{}
	if found, err := store.BaseLoadOneByQuery(tx, query, entity); !found || err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *identityStoreImpl) DeleteById(ctx boltz.MutateContext, id string) error {
	for _, apiSessionId := range store.GetRelatedEntitiesIdList(ctx.Tx(), id, FieldIdentityApiSessions) {
		if err := store.stores.apiSession.DeleteById(ctx, apiSessionId); err != nil {
			return err
		}
	}

	for _, enrollmentId := range store.GetRelatedEntitiesIdList(ctx.Tx(), id, FieldIdentityEnrollments) {
		if err := store.stores.enrollment.DeleteById(ctx, enrollmentId); err != nil {
			return err
		}
	}

	for _, authenticatorId := range store.GetRelatedEntitiesIdList(ctx.Tx(), id, FieldIdentityAuthenticators) {
		if err := store.stores.authenticator.DeleteById(ctx, authenticatorId); err != nil {
			return err
		}
	}

	if entity, _ := store.LoadOneById(ctx.Tx(), id); entity != nil {
		// Remove entity from IdentityRoles in edge router policies
		if err := store.deleteEntityReferences(ctx.Tx(), entity, store.stores.edgeRouterPolicy.symbolIdentityRoles); err != nil {
			return err
		}
		// Remove entity from IdentityRoles in service policies
		if err := store.deleteEntityReferences(ctx.Tx(), entity, store.stores.servicePolicy.symbolIdentityRoles); err != nil {
			return err
		}
	}

	if err := store.removeServiceConfigs(ctx.Tx(), id, func(_, _, _ string) bool { return true }); err != nil {
		return err
	}

	return store.baseStore.DeleteById(ctx, id)
}

func (store *identityStoreImpl) AssignServiceConfigs(tx *bbolt.Tx, identityId string, serviceConfigs ...ServiceConfig) error {
	entityBucket := store.GetEntityBucket(tx, []byte(identityId))
	if entityBucket == nil {
		return util.NewNotFoundError(store.GetSingularEntityType(), "id", identityId)
	}
	configsBucket := entityBucket.GetOrCreateBucket(FieldIdentityServiceConfigs)
	if configsBucket.HasError() {
		return configsBucket.GetError()
	}
	configTypes := map[string]struct{}{}
	for _, serviceConfig := range serviceConfigs {
		config, err := store.stores.config.LoadOneById(tx, serviceConfig.ConfigId)
		if err != nil {
			return err
		}

		if _, ok := configTypes[config.Type]; ok {
			return errors.Errorf("multiple service configs provided for identity %v of config type %v", identityId, config.Type)
		}
		configTypes[config.Type] = struct{}{}

		serviceBucket := configsBucket.GetOrCreateBucket(serviceConfig.ServiceId)
		if serviceBucket.HasError() {
			return serviceBucket.GetError()
		}

		// un-index old value
		if currentConfigId := serviceBucket.GetString(config.Type); currentConfigId != nil {
			if err := store.stores.config.identityServicesLinks.RemoveCompoundLink(tx, *currentConfigId, ss(identityId, serviceConfig.ServiceId)); err != nil {
				return err
			}
		}
		// set new value
		if serviceBucket.SetString(config.Type, config.Id, nil).HasError() {
			return serviceBucket.GetError()
		}

		// index new value
		if err := store.stores.config.identityServicesLinks.AddCompoundLink(tx, config.Id, ss(identityId, serviceConfig.ServiceId)); err != nil {
			return err
		}
	}
	return nil
}

func (store *identityStoreImpl) RemoveServiceConfigs(tx *bbolt.Tx, identityId string, serviceConfigs ...ServiceConfig) error {
	// could make this more efficient with maps, but only necessary if we have large number of overrides, which seems unlikely
	// optimize later, if necessary
	return store.removeServiceConfigs(tx, identityId, func(serviceId, _, configId string) bool {
		if len(serviceConfigs) == 0 {
			return true
		}
		for _, config := range serviceConfigs {
			if config.ServiceId == serviceId && config.ConfigId == configId {
				return true
			}
		}
		return false
	})
}

func (store *identityStoreImpl) removeServiceConfigs(tx *bbolt.Tx, identityId string, removeFilter func(serviceId, configTypeId, configId string) bool) error {
	entityBucket := store.GetEntityBucket(tx, []byte(identityId))
	if entityBucket == nil {
		return util.NewNotFoundError(store.GetSingularEntityType(), "id", identityId)
	}
	configsBucket := entityBucket.GetBucket(FieldIdentityServiceConfigs)
	if configsBucket == nil {
		// no service configs found, nothing to do, bail out
		return nil
	}

	servicesCursor := configsBucket.Cursor()
	for sKey, _ := servicesCursor.First(); sKey != nil; sKey, _ = servicesCursor.Next() {
		serviceId := string(sKey)
		serviceBucket := configsBucket.GetBucket(serviceId)
		if serviceBucket == nil {
			// doesn't exist, nothing to do, continue
			continue
		}
		cursor := serviceBucket.Cursor()
		for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
			configTypeId := string(k)
			configId := *boltz.FieldToString(boltz.GetTypeAndValue(v))
			if removeFilter(serviceId, configTypeId, configId) {
				if err := cursor.Delete(); err != nil {
					return err
				}
				if err := store.stores.config.identityServicesLinks.RemoveCompoundLink(tx, configId, ss(identityId, serviceId)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (store *identityStoreImpl) GetServiceConfigs(tx *bbolt.Tx, identityId string) ([]ServiceConfig, error) {
	entityBucket := store.GetEntityBucket(tx, []byte(identityId))
	if entityBucket == nil {
		return nil, util.NewNotFoundError(store.GetSingularEntityType(), "id", identityId)
	}
	var result []ServiceConfig
	configsBucket := entityBucket.GetBucket(FieldIdentityServiceConfigs)
	if configsBucket == nil {
		// no service configs found, nothing to do, bail out
		return result, nil
	}

	servicesCursor := configsBucket.Cursor()
	for sKey, _ := servicesCursor.First(); sKey != nil; sKey, _ = servicesCursor.Next() {
		serviceId := string(sKey)
		serviceBucket := configsBucket.GetBucket(serviceId)
		if serviceBucket == nil {
			// doesn't exist, nothing to do, continue
			continue
		}
		cursor := serviceBucket.Cursor()
		for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
			configId := *boltz.FieldToString(boltz.GetTypeAndValue(v))
			result = append(result, ServiceConfig{ServiceId: serviceId, ConfigId: configId})
		}
	}
	return result, nil
}

func (store *identityStoreImpl) LoadServiceConfigsByServiceAndType(tx *bbolt.Tx, identityId string, configTypes map[string]struct{}) map[string]map[string]map[string]interface{} {
	if len(configTypes) == 0 {
		return nil
	}
	result := map[string]map[string]map[string]interface{}{}

	entityBucket := store.GetEntityBucket(tx, []byte(identityId))
	if entityBucket == nil {
		return result
	}
	configsBucket := entityBucket.GetBucket(FieldIdentityServiceConfigs)
	if configsBucket == nil {
		return result
	}

	servicesCursor := configsBucket.Cursor()
	for sKey, _ := servicesCursor.First(); sKey != nil; sKey, _ = servicesCursor.Next() {
		serviceId := string(sKey)
		serviceBucket := configsBucket.GetBucket(serviceId)
		if serviceBucket == nil {
			// doesn't exist, nothing to do, continue
			continue
		}

		_, wantAll := configTypes["all"]
		cursor := serviceBucket.Cursor()
		for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
			configTypeId := string(k)
			configId := *boltz.FieldToString(boltz.GetTypeAndValue(v))

			wantsType := wantAll
			if !wantsType {
				_, wantsType = configTypes[configTypeId]
			}
			if wantsType {
				if config, _ := store.stores.config.LoadOneById(tx, configId); config != nil {
					serviceMap, ok := result[serviceId]
					if !ok {
						serviceMap = map[string]map[string]interface{}{}
						result[serviceId] = serviceMap
					}
					serviceMap[configTypeId] = config.Data
				} else {
					pfxlog.Logger().Errorf("config id %v was referenced by identity %v, but no longer exists", configId, identityId)
					_ = cursor.Delete()
				}
			}
		}
	}

	return result
}
