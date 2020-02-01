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
	"github.com/netfoundry/ziti-foundation/storage/ast"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/netfoundry/ziti-foundation/util/errorz"
	"go.etcd.io/bbolt"
)

const (
	FieldIdentityType           = "type"
	FieldIdentityApiSessions    = "apiSessions"
	FieldIdentityIsDefaultAdmin = "isDefaultAdmin"
	FieldIdentityIsAdmin        = "isAdmin"
	FieldIdentityEnrollments    = "enrollments"
	FieldIdentityAuthenticators = "authenticators"
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
	symbolAppwans            boltz.EntitySymbol
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
	store.symbolAppwans = store.AddFkSetSymbol(EntityTypeAppwans, store.stores.appwan)
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
	store.AddLinkCollection(store.symbolAppwans, store.stores.appwan.symbolIdentities)
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

	return store.baseStore.DeleteById(ctx, id)
}
