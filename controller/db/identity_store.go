/*
	Copyright NetFoundry Inc.

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

package db

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/common/eid"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"strings"
	"time"
)

const (
	FieldIdentityType           = "type"
	FieldIdentityIsDefaultAdmin = "isDefaultAdmin"
	FieldIdentityIsAdmin        = "isAdmin"
	FieldIdentityEnrollments    = "enrollments"
	FieldIdentityAuthenticators = "authenticators"
	FieldIdentityServiceConfigs = "serviceConfigs"

	FieldIdentityEnvInfoArch       = "envInfoArch"
	FieldIdentityEnvInfoOs         = "envInfoOs"
	FieldIdentityEnvInfoOsRelease  = "envInfoRelease"
	FieldIdentityEnvInfoOsVersion  = "envInfoVersion"
	FieldIdentityEnvInfoDomain     = "envInfoDomain"
	FieldIdentityEnvInfoHostname   = "envInfoHostname"
	FieldIdentitySdkInfoBranch     = "sdkInfoBranch"
	FieldIdentitySdkInfoRevision   = "sdkInfoRevision"
	FieldIdentitySdkInfoType       = "sdkInfoType"
	FieldIdentitySdkInfoVersion    = "sdkInfoVersion"
	FieldIdentitySdkInfoAppId      = "sdkInfoAppId"
	FieldIdentitySdkInfoAppVersion = "sdkInfoAppVersion"

	FieldIdentityBindServices              = "bindServices"
	FieldIdentityDialServices              = "dialServices"
	FieldIdentityDefaultHostingPrecedence  = "defaultHostingPrecedence"
	FieldIdentityDefaultHostingCost        = "defaultHostingCost"
	FieldIdentityServiceHostingPrecedences = "serviceHostingPrecedences"
	FieldIdentityServiceHostingCosts       = "serviceHostingCosts"
	FieldIdentityAppData                   = "appData"
	FieldIdentityAuthPolicyId              = "authPolicyId"
	FieldIdentityExternalId                = "externalId"
	FieldIdentityDisabledAt                = "disabledAt"
	FieldIdentityDisabledUntil             = "disabledUntil"
)

func newIdentity(name string, identityTypeId string, roleAttributes ...string) *Identity {
	return &Identity{
		BaseExtEntity:  boltz.BaseExtEntity{Id: eid.New()},
		Name:           name,
		IdentityTypeId: identityTypeId,
		RoleAttributes: roleAttributes,
	}
}

type EnvInfo struct {
	Arch      string `json:"arch"`
	Os        string `json:"os"`
	OsRelease string `json:"osRelease"`
	OsVersion string `json:"osVersion"`
	Domain    string `json:"domain"`
	Hostname  string `json:"hostname"`
}

type SdkInfo struct {
	Branch     string `json:"branch"`
	Revision   string `json:"revision"`
	Type       string `json:"type"`
	Version    string `json:"version"`
	AppId      string `json:"appId"`
	AppVersion string `json:"appVersion"`
}

type Identity struct {
	boltz.BaseExtEntity
	Name                      string                       `json:"name"`
	IdentityTypeId            string                       `json:"identityTypeId"`
	IsDefaultAdmin            bool                         `json:"isDefaultAdmin"`
	IsAdmin                   bool                         `json:"isAdmin"`
	Enrollments               []string                     `json:"enrollments"`
	Authenticators            []string                     `json:"authenticators"`
	RoleAttributes            []string                     `json:"roleAttributes"`
	SdkInfo                   *SdkInfo                     `json:"sdkInfo"`
	EnvInfo                   *EnvInfo                     `json:"envInfo"`
	DefaultHostingPrecedence  ziti.Precedence              `json:"defaultHostingPrecedence"`
	DefaultHostingCost        uint16                       `json:"defaultHostingCost"`
	ServiceHostingPrecedences map[string]ziti.Precedence   `json:"serviceHostingPrecedences"`
	ServiceHostingCosts       map[string]uint16            `json:"serviceHostingCosts"`
	AppData                   map[string]interface{}       `json:"appData"`
	AuthPolicyId              string                       `json:"authPolicyId"`
	ExternalId                *string                      `json:"externalId"`
	DisabledAt                *time.Time                   `json:"disabledAt"`
	DisabledUntil             *time.Time                   `json:"disabledUntil"`
	Disabled                  bool                         `json:"disabled"`
	ServiceConfigs            map[string]map[string]string `json:"serviceConfigs"`
}

func (entity *Identity) GetEntityType() string {
	return EntityTypeIdentities
}

func (entity *Identity) GetName() string {
	return entity.Name
}

var identityFieldMappings = map[string]string{FieldIdentityType: "identityTypeId"}

var _ IdentityStore = (*identityStoreImpl)(nil)

type IdentityStore interface {
	NameIndexed
	Store[*Identity]

	GetRoleAttributesIndex() boltz.SetReadIndex
	GetRoleAttributesCursorProvider(values []string, semantic string) (ast.SetCursorProvider, error)

	LoadServiceConfigsByServiceAndType(tx *bbolt.Tx, identityId string, configTypes map[string]struct{}) map[string]map[string]map[string]interface{}
	GetIdentityServicesCursorProvider(identityId string) ast.SetCursorProvider
}

func newIdentityStore(stores *stores) *identityStoreImpl {
	store := &identityStoreImpl{}
	store.baseStore = newBaseStore[*Identity](stores, store)
	store.InitImpl(store)
	return store
}

type identityStoreImpl struct {
	*baseStore[*Identity]

	indexName           boltz.ReadIndex
	indexRoleAttributes boltz.SetReadIndex

	symbolRoleAttributes boltz.EntitySetSymbol
	symbolAuthenticators boltz.EntitySetSymbol
	symbolIdentityTypeId boltz.EntitySymbol
	symbolAuthPolicyId   boltz.EntitySymbol
	symbolEnrollments    boltz.EntitySetSymbol

	symbolEdgeRouterPolicies boltz.EntitySetSymbol
	symbolServicePolicies    boltz.EntitySetSymbol
	symbolEdgeRouters        boltz.EntitySetSymbol
	symbolBindServices       boltz.EntitySetSymbol
	symbolDialServices       boltz.EntitySetSymbol

	edgeRoutersCollection  boltz.RefCountedLinkCollection
	bindServicesCollection boltz.RefCountedLinkCollection
	dialServicesCollection boltz.RefCountedLinkCollection
	symbolExternalId       boltz.EntitySymbol
	externalIdIndex        boltz.ReadIndex
}

func (store *identityStoreImpl) GetRoleAttributesIndex() boltz.SetReadIndex {
	return store.indexRoleAttributes
}

func (store *identityStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()

	store.symbolRoleAttributes = store.AddPublicSetSymbol(FieldRoleAttributes, ast.NodeTypeString)
	store.indexRoleAttributes = store.AddSetIndex(store.symbolRoleAttributes)

	store.indexName = store.addUniqueNameField()
	store.symbolEdgeRouters = store.AddFkSetSymbol(EntityTypeRouters, store.stores.edgeRouter)
	store.symbolBindServices = store.AddFkSetSymbol(FieldIdentityBindServices, store.stores.edgeService)
	store.symbolDialServices = store.AddFkSetSymbol(FieldIdentityDialServices, store.stores.edgeService)
	store.symbolEdgeRouterPolicies = store.AddFkSetSymbol(EntityTypeEdgeRouterPolicies, store.stores.edgeRouterPolicy)
	store.symbolServicePolicies = store.AddFkSetSymbol(EntityTypeServicePolicies, store.stores.servicePolicy)
	store.symbolEnrollments = store.AddFkSetSymbol(FieldIdentityEnrollments, store.stores.enrollment)
	store.symbolAuthenticators = store.AddFkSetSymbol(FieldIdentityAuthenticators, store.stores.authenticator)
	store.symbolExternalId = store.AddSymbol(FieldIdentityExternalId, ast.NodeTypeString)
	store.externalIdIndex = store.AddNullableUniqueIndex(store.symbolExternalId)

	store.symbolIdentityTypeId = store.AddFkSymbol(FieldIdentityType, store.stores.identityType)
	store.symbolAuthPolicyId = store.AddFkSymbol(FieldIdentityAuthPolicyId, store.stores.authPolicy)

	store.AddFkConstraint(store.symbolAuthPolicyId, true, boltz.CascadeNone)

	store.AddSymbol(FieldIdentityIsAdmin, ast.NodeTypeBool)
	store.AddSymbol(FieldIdentityIsDefaultAdmin, ast.NodeTypeBool)

	store.indexRoleAttributes.AddListener(store.rolesChanged)
}

func (store *identityStoreImpl) initializeLinked() {
	store.AddLinkCollection(store.symbolEdgeRouterPolicies, store.stores.edgeRouterPolicy.symbolIdentities)
	store.AddLinkCollection(store.symbolServicePolicies, store.stores.servicePolicy.symbolIdentities)

	store.edgeRoutersCollection = store.AddRefCountedLinkCollection(store.symbolEdgeRouters, store.stores.edgeRouter.symbolIdentities)
	store.bindServicesCollection = store.AddRefCountedLinkCollection(store.symbolBindServices, store.stores.edgeService.symbolBindIdentities)
	store.dialServicesCollection = store.AddRefCountedLinkCollection(store.symbolDialServices, store.stores.edgeService.symbolDialIdentities)
}

func (store *identityStoreImpl) NewEntity() *Identity {
	return &Identity{}
}

func (store *identityStoreImpl) FillEntity(entity *Identity, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
	entity.IdentityTypeId = bucket.GetStringWithDefault(FieldIdentityType, "")
	entity.AuthPolicyId = bucket.GetStringWithDefault(FieldIdentityAuthPolicyId, DefaultAuthPolicyId)
	entity.IsDefaultAdmin = bucket.GetBoolWithDefault(FieldIdentityIsDefaultAdmin, false)
	entity.IsAdmin = bucket.GetBoolWithDefault(FieldIdentityIsAdmin, false)
	entity.Authenticators = bucket.GetStringList(FieldIdentityAuthenticators)
	entity.Enrollments = bucket.GetStringList(FieldIdentityEnrollments)
	entity.RoleAttributes = bucket.GetStringList(FieldRoleAttributes)
	entity.DefaultHostingPrecedence = ziti.Precedence(bucket.GetInt32WithDefault(FieldIdentityDefaultHostingPrecedence, 0))
	entity.DefaultHostingCost = uint16(bucket.GetInt32WithDefault(FieldIdentityDefaultHostingCost, 0))
	entity.AppData = bucket.GetMap(FieldIdentityAppData)
	entity.ExternalId = bucket.GetString(FieldIdentityExternalId)

	entity.Disabled = false
	entity.DisabledAt = bucket.GetTime(FieldIdentityDisabledAt)
	entity.DisabledUntil = bucket.GetTime(FieldIdentityDisabledUntil)

	if entity.DisabledAt != nil {
		if entity.DisabledUntil == nil || entity.DisabledUntil.After(time.Now()) {
			entity.Disabled = true
		}
	}

	entity.SdkInfo = &SdkInfo{
		Branch:     bucket.GetStringWithDefault(FieldIdentitySdkInfoBranch, ""),
		Revision:   bucket.GetStringWithDefault(FieldIdentitySdkInfoRevision, ""),
		Type:       bucket.GetStringWithDefault(FieldIdentitySdkInfoType, ""),
		Version:    bucket.GetStringWithDefault(FieldIdentitySdkInfoVersion, ""),
		AppId:      bucket.GetStringWithDefault(FieldIdentitySdkInfoAppId, ""),
		AppVersion: bucket.GetStringWithDefault(FieldIdentitySdkInfoAppVersion, ""),
	}

	entity.EnvInfo = &EnvInfo{
		Arch:      bucket.GetStringWithDefault(FieldIdentityEnvInfoArch, ""),
		Os:        bucket.GetStringWithDefault(FieldIdentityEnvInfoOs, ""),
		OsRelease: bucket.GetStringWithDefault(FieldIdentityEnvInfoOsRelease, ""),
		OsVersion: bucket.GetStringWithDefault(FieldIdentityEnvInfoOsVersion, ""),
		Domain:    bucket.GetStringWithDefault(FieldIdentityEnvInfoDomain, ""),
		Hostname:  bucket.GetStringWithDefault(FieldIdentityEnvInfoHostname, ""),
	}

	entity.ServiceHostingPrecedences = map[string]ziti.Precedence{}
	for k, v := range bucket.GetMap(FieldIdentityServiceHostingPrecedences) {
		entity.ServiceHostingPrecedences[k] = ziti.Precedence(v.(int32))
	}

	entity.ServiceHostingCosts = map[string]uint16{}
	for k, v := range bucket.GetMap(FieldIdentityServiceHostingCosts) {
		entity.ServiceHostingCosts[k] = uint16(v.(int32))
	}

	store.fillServiceConfig(entity, bucket)
}

func (store *identityStoreImpl) fillServiceConfig(entity *Identity, entityBucket *boltz.TypedBucket) {
	configsBucket := entityBucket.GetBucket(FieldIdentityServiceConfigs)
	if configsBucket != nil {
		servicesCursor := configsBucket.Cursor()
		for sKey, _ := servicesCursor.First(); sKey != nil; sKey, _ = servicesCursor.Next() {
			serviceMap := map[string]string{}
			serviceId := string(sKey)
			serviceBucket := configsBucket.GetBucket(serviceId)
			if serviceBucket != nil {
				cursor := serviceBucket.Cursor()
				for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
					configType := string(k)
					configId := *boltz.FieldToString(boltz.GetTypeAndValue(v))
					serviceMap[configType] = configId
				}
			}
			if len(serviceMap) > 0 {
				if entity.ServiceConfigs == nil {
					entity.ServiceConfigs = map[string]map[string]string{}
				}
				entity.ServiceConfigs[serviceId] = serviceMap
			}
		}
	}
}

func (store *identityStoreImpl) PersistEntity(entity *Identity, ctx *boltz.PersistContext) {
	ctx.WithFieldOverrides(identityFieldMappings)

	entity.SetBaseValues(ctx)

	ctx.SetString(FieldName, entity.Name)
	ctx.SetBool(FieldIdentityIsDefaultAdmin, entity.IsDefaultAdmin)
	ctx.SetBool(FieldIdentityIsAdmin, entity.IsAdmin)
	if oldValue, changed := ctx.GetAndSetString(FieldIdentityType, entity.IdentityTypeId); changed {
		if oldValue != nil && *oldValue == RouterIdentityType {
			ctx.Bucket.SetError(errors.New("cannot change type of router identity"))
		}
	}
	if strings.TrimSpace(entity.AuthPolicyId) == "" {
		entity.AuthPolicyId = DefaultAuthPolicyId
	}
	ctx.SetString(FieldIdentityAuthPolicyId, entity.AuthPolicyId)
	store.validateRoleAttributes(entity.RoleAttributes, ctx.Bucket)
	ctx.SetStringList(FieldRoleAttributes, entity.RoleAttributes)
	ctx.SetInt32(FieldIdentityDefaultHostingPrecedence, int32(entity.DefaultHostingPrecedence))
	ctx.SetInt32(FieldIdentityDefaultHostingCost, int32(entity.DefaultHostingCost))
	ctx.Bucket.PutMap(FieldIdentityAppData, entity.AppData, ctx.FieldChecker, false)

	ctx.SetTimeP(FieldIdentityDisabledAt, entity.DisabledAt)
	ctx.SetTimeP(FieldIdentityDisabledUntil, entity.DisabledUntil)

	//treat empty string and white space like nil
	if entity.ExternalId != nil && len(strings.TrimSpace(*entity.ExternalId)) == 0 {
		entity.ExternalId = nil
	}
	ctx.SetStringP(FieldIdentityExternalId, entity.ExternalId)

	if entity.EnvInfo != nil {
		ctx.SetString(FieldIdentityEnvInfoArch, entity.EnvInfo.Arch)
		ctx.SetString(FieldIdentityEnvInfoOs, entity.EnvInfo.Os)
		ctx.SetString(FieldIdentityEnvInfoOsRelease, entity.EnvInfo.OsRelease)
		ctx.SetString(FieldIdentityEnvInfoOsVersion, entity.EnvInfo.OsVersion)
		ctx.SetString(FieldIdentityEnvInfoDomain, entity.EnvInfo.Domain)
		ctx.SetString(FieldIdentityEnvInfoHostname, entity.EnvInfo.Hostname)
	}

	if entity.SdkInfo != nil {
		ctx.SetString(FieldIdentitySdkInfoBranch, entity.SdkInfo.Branch)
		ctx.SetString(FieldIdentitySdkInfoRevision, entity.SdkInfo.Revision)
		ctx.SetString(FieldIdentitySdkInfoType, entity.SdkInfo.Type)
		ctx.SetString(FieldIdentitySdkInfoVersion, entity.SdkInfo.Version)
		ctx.SetString(FieldIdentitySdkInfoAppId, entity.SdkInfo.AppId)
		ctx.SetString(FieldIdentitySdkInfoAppVersion, entity.SdkInfo.AppVersion)
	}

	serviceStore := store.stores.service

	if ctx.ProceedWithSet(FieldIdentityServiceHostingPrecedences) {
		mapBucket, err := ctx.Bucket.EmptyBucket(FieldIdentityServiceHostingPrecedences)
		if !ctx.Bucket.SetError(err) {
			for k, v := range entity.ServiceHostingPrecedences {
				if !serviceStore.IsEntityPresent(ctx.Tx(), k) {
					ctx.Bucket.SetError(boltz.NewNotFoundError(serviceStore.GetEntityType(), "id", k))
					return
				}
				mapBucket.SetInt32(k, int32(v), nil)
			}
			ctx.Bucket.SetError(mapBucket.Err)
		}
	}

	if ctx.ProceedWithSet(FieldIdentityServiceHostingCosts) {
		mapBucket, err := ctx.Bucket.EmptyBucket(FieldIdentityServiceHostingCosts)
		if !ctx.Bucket.SetError(err) {
			for k, v := range entity.ServiceHostingCosts {
				if !serviceStore.IsEntityPresent(ctx.Tx(), k) {
					ctx.Bucket.SetError(boltz.NewNotFoundError(serviceStore.GetEntityType(), "id", k))
					return
				}

				mapBucket.SetInt32(k, int32(v), nil)
			}
			ctx.Bucket.SetError(mapBucket.Err)
		}
	}

	// index change won't fire if we don't have any roles on create, but we need to evaluate if we match any #all roles
	if ctx.IsCreate && len(entity.RoleAttributes) == 0 {
		store.rolesChanged(ctx.MutateContext, []byte(entity.Id), nil, nil, ctx.Bucket)
	}

	store.persistServiceConfigs(entity, ctx)
}

func (store *identityStoreImpl) persistServiceConfigs(entity *Identity, ctx *boltz.PersistContext) {
	if !ctx.ProceedWithSet(FieldIdentityServiceConfigs) {
		return
	}

	entityBucket := ctx.Bucket

	configsBucket := entityBucket.GetOrCreateBucket(FieldIdentityServiceConfigs)
	if configsBucket.HasError() {
		return
	}

	var serviceEvents []*ServiceEvent

	foundServices := map[string]struct{}{}

	servicesCursor := configsBucket.Cursor()
	for sKey, _ := servicesCursor.First(); sKey != nil; sKey, _ = servicesCursor.Next() {
		serviceId := string(sKey)
		serviceMap := map[string]string{}
		foundServices[serviceId] = struct{}{}
		serviceBucket := configsBucket.GetOrCreateBucket(serviceId)

		cursor := serviceBucket.Cursor()
		for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
			configType := string(k)
			configId := *boltz.FieldToString(boltz.GetTypeAndValue(v))
			serviceMap[configType] = configId
		}

		serviceUpdated := false
		updated, serviceFound := entity.ServiceConfigs[serviceId]

		for configType, configId := range serviceMap {
			newConfigId, configTypeFound := updated[configType]
			if !configTypeFound || configId != newConfigId {
				// un-index old value
				if err := store.stores.config.identityServicesLinks.RemoveCompoundLink(ctx.Tx(), configId, ss(entity.Id, serviceId)); err != nil {
					ctx.Bucket.SetError(err)
					return
				}
				serviceUpdated = true
			}
			if !configTypeFound {
				serviceBucket.DeleteValue([]byte(configType))
			}
		}

		for configType, configId := range updated {
			oldConfigId, ok := serviceMap[configType]
			if !ok || configId != oldConfigId {
				serviceUpdated = true
				// set new value
				serviceBucket.SetString(configType, configId, nil)

				// index new value
				if err := store.stores.config.identityServicesLinks.AddCompoundLink(ctx.Tx(), configId, ss(entity.Id, serviceId)); err != nil {
					serviceBucket.SetError(err)
					return
				}
			}
		}

		if !serviceFound {
			// no overrides for service, delete existing
			configsBucket.DeleteEntity(serviceId)
		}

		if serviceUpdated {
			serviceEvents = append(serviceEvents, &ServiceEvent{
				Type:       ServiceUpdated,
				IdentityId: entity.Id,
				ServiceId:  serviceId,
			})
		}
	}

	// handle new service mappings
	for serviceId, configMappings := range entity.ServiceConfigs {
		if _, ok := foundServices[serviceId]; ok {
			continue
		}

		serviceBucket := configsBucket.GetOrCreateBucket(serviceId)
		for configType, configId := range configMappings {
			// set new value
			serviceBucket.SetString(configType, configId, nil)

			// index new value
			if err := store.stores.config.identityServicesLinks.AddCompoundLink(ctx.Tx(), configId, ss(entity.Id, serviceId)); err != nil {
				serviceBucket.SetError(err)
				return
			}
		}

		serviceEvents = append(serviceEvents, &ServiceEvent{
			Type:       ServiceUpdated,
			IdentityId: entity.Id,
			ServiceId:  serviceId,
		})
	}

	ctx.Tx().OnCommit(func() {
		ServiceEvents.dispatchEventsAsync(serviceEvents)
	})
}

func (store *identityStoreImpl) rolesChanged(mutateCtx boltz.MutateContext, rowId []byte, _ []boltz.FieldTypeAndValue, new []boltz.FieldTypeAndValue, holder errorz.ErrorHolder) {
	ctx := &roleAttributeChangeContext{
		mutateCtx:             mutateCtx,
		rolesSymbol:           store.stores.edgeRouterPolicy.symbolIdentityRoles,
		linkCollection:        store.stores.edgeRouterPolicy.identityCollection,
		relatedLinkCollection: store.stores.edgeRouterPolicy.edgeRouterCollection,
		denormLinkCollection:  store.edgeRoutersCollection,
		ErrorHolder:           holder,
	}
	UpdateRelatedRoles(ctx, rowId, new, store.stores.edgeRouterPolicy.symbolSemantic)

	ctx = &roleAttributeChangeContext{
		mutateCtx:             mutateCtx,
		rolesSymbol:           store.stores.servicePolicy.symbolIdentityRoles,
		linkCollection:        store.stores.servicePolicy.identityCollection,
		relatedLinkCollection: store.stores.servicePolicy.serviceCollection,
		ErrorHolder:           holder,
	}
	store.updateServicePolicyRelatedRoles(ctx, rowId, new)
}

func (store *identityStoreImpl) GetNameIndex() boltz.ReadIndex {
	return store.indexName
}

func (store *identityStoreImpl) DeleteById(ctx boltz.MutateContext, id string) error {
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

	if entity, _ := store.LoadById(ctx.Tx(), id); entity != nil {
		if entity.IsDefaultAdmin {
			return errorz.NewEntityCanNotBeDeleted()
		}
		if entity.IdentityTypeId == RouterIdentityType {
			if !ctx.IsSystemContext() {
				router, err := store.stores.router.FindByName(ctx.Tx(), entity.Name)
				if err != nil {
					return errorz.NewEntityCanNotBeDeletedFrom(err)
				}
				if router != nil {
					err = errors.Errorf("cannot delete router identity %v until associated router is deleted", entity.Name)
					return errorz.NewEntityCanNotBeDeletedFrom(err)
				}
			}
		}
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

func (store *identityStoreImpl) removeServiceConfigs(tx *bbolt.Tx, identityId string, removeFilter func(serviceId, configTypeId, configId string) bool) error {
	entityBucket := store.GetEntityBucket(tx, []byte(identityId))
	if entityBucket == nil {
		return boltz.NewNotFoundError(store.GetSingularEntityType(), "id", identityId)
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
				if config, _ := store.stores.config.LoadById(tx, configId); config != nil {
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

func (store *identityStoreImpl) GetRoleAttributesCursorProvider(values []string, semantic string) (ast.SetCursorProvider, error) {
	return store.getRoleAttributesCursorProvider(store.indexRoleAttributes, values, semantic)
}

func (store *identityStoreImpl) GetIdentityServicesCursorProvider(identityId string) ast.SetCursorProvider {
	provider := &IdentityServicesCursorProvider{
		store:      store,
		identityId: identityId,
	}
	return provider.Cursor
}

type IdentityServicesCursorProvider struct {
	store      *identityStoreImpl
	identityId string
}

func (self *IdentityServicesCursorProvider) Cursor(tx *bbolt.Tx, forward bool) ast.SetCursor {
	fst := self.store.dialServicesCollection.IterateLinks(tx, []byte(self.identityId), forward)
	snd := self.store.bindServicesCollection.IterateLinks(tx, []byte(self.identityId), forward)
	if !fst.IsValid() {
		return snd
	}
	if !snd.IsValid() {
		return fst
	}
	return ast.NewUnionSetCursor(fst, snd, forward)
}
