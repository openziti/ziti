/*
	Copyright NetFoundry, Inc.

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
	"time"
)

const (
	FieldEdgeRouterFingerprint         = "fingerprint"
	FieldEdgeRouterCertPEM             = "certPem"
	FieldEdgeRouterCluster             = "cluster"
	FieldEdgeRouterIsVerified          = "isVerified"
	FieldEdgeRouterEnrollmentToken     = "enrollmentToken"
	FieldEdgeRouterHostname            = "hostname"
	FieldEdgeRouterEnrollmentJwt       = "enrollmentJwt"
	FieldEdgeRouterEnrollmentCreatedAt = "enrollmentCreatedAt"
	FieldEdgeRouterEnrollmentExpiresAt = "enrollmentExpiresAt"
	FieldEdgeRouterProtocols           = "protocols"
)

func newEdgeRouter(name string, roleAttributes ...string) *EdgeRouter {
	return &EdgeRouter{
		BaseExtEntity:  boltz.BaseExtEntity{Id: uuid.New().String()},
		Name:           name,
		RoleAttributes: roleAttributes,
	}
}

type EdgeRouter struct {
	boltz.BaseExtEntity
	Name                string
	ClusterId           *string
	IsVerified          bool
	Fingerprint         *string
	CertPem             *string
	EnrollmentToken     *string
	Hostname            *string
	EnrollmentJwt       *string
	EnrollmentCreatedAt *time.Time
	EnrollmentExpiresAt *time.Time
	EdgeRouterProtocols map[string]string
	RoleAttributes      []string
}

var edgeRouterFieldMappings = map[string]string{FieldEdgeRouterCluster: "clusterId"}

func (entity *EdgeRouter) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
	entity.Fingerprint = bucket.GetString(FieldEdgeRouterFingerprint)
	entity.CertPem = bucket.GetString(FieldEdgeRouterCertPEM)
	entity.ClusterId = bucket.GetString(FieldEdgeRouterCluster)
	entity.IsVerified = bucket.GetBoolWithDefault(FieldEdgeRouterIsVerified, false)

	entity.EnrollmentToken = bucket.GetString(FieldEdgeRouterEnrollmentToken)
	entity.Hostname = bucket.GetString(FieldEdgeRouterHostname)
	entity.EnrollmentJwt = bucket.GetString(FieldEdgeRouterEnrollmentJwt)
	entity.EnrollmentCreatedAt = bucket.GetTime(FieldEdgeRouterEnrollmentCreatedAt)
	entity.EnrollmentExpiresAt = bucket.GetTime(FieldEdgeRouterEnrollmentExpiresAt)
	entity.EdgeRouterProtocols = toStringStringMap(bucket.GetMap(FieldEdgeRouterProtocols))
	entity.RoleAttributes = bucket.GetStringList(FieldRoleAttributes)
}

func (entity *EdgeRouter) SetValues(ctx *boltz.PersistContext) {
	ctx.WithFieldOverrides(edgeRouterFieldMappings)

	entity.SetBaseValues(ctx)
	store := ctx.Store.(*edgeRouterStoreImpl)
	ctx.SetString(FieldName, entity.Name)
	ctx.SetStringP(FieldEdgeRouterFingerprint, entity.Fingerprint)
	ctx.SetStringP(FieldEdgeRouterCertPEM, entity.CertPem)
	ctx.SetStringP(FieldEdgeRouterCluster, entity.ClusterId)
	ctx.SetBool(FieldEdgeRouterIsVerified, entity.IsVerified)
	ctx.SetStringP(FieldEdgeRouterEnrollmentToken, entity.EnrollmentToken)
	ctx.SetStringP(FieldEdgeRouterHostname, entity.Hostname)
	ctx.SetStringP(FieldEdgeRouterEnrollmentJwt, entity.EnrollmentJwt)
	ctx.SetTimeP(FieldEdgeRouterEnrollmentCreatedAt, entity.EnrollmentCreatedAt)
	ctx.SetTimeP(FieldEdgeRouterEnrollmentExpiresAt, entity.EnrollmentExpiresAt)
	ctx.SetMap(FieldEdgeRouterProtocols, toStringInterfaceMap(entity.EdgeRouterProtocols))
	ctx.SetStringList(FieldRoleAttributes, entity.RoleAttributes)

	// index change won't fire if we don't have any roles on create, but we need to evaluate if we match any #all roles
	if ctx.IsCreate && len(entity.RoleAttributes) == 0 {
		store.rolesChanged(ctx.Bucket.Tx(), []byte(entity.Id), nil, nil, ctx.Bucket)
	}
}

func (entity *EdgeRouter) GetEntityType() string {
	return EntityTypeEdgeRouters
}

func (entity *EdgeRouter) GetName() string {
	return entity.Name
}

type EdgeRouterStore interface {
	NameIndexedStore
	LoadOneById(tx *bbolt.Tx, id string) (*EdgeRouter, error)
	LoadOneByName(tx *bbolt.Tx, id string) (*EdgeRouter, error)
	GetRoleAttributesIndex() boltz.SetReadIndex
	GetRoleAttributesCursorProvider(values []string, semantic string) (ast.SetCursorProvider, error)
}

func newEdgeRouterStore(stores *stores) *edgeRouterStoreImpl {
	store := &edgeRouterStoreImpl{
		baseStore: newBaseStore(stores, EntityTypeEdgeRouters),
	}
	store.InitImpl(store)
	return store
}

type edgeRouterStoreImpl struct {
	*baseStore

	indexName           boltz.ReadIndex
	indexRoleAttributes boltz.SetReadIndex

	symbolEdgeRouterPolicies        boltz.EntitySetSymbol
	symbolServiceEdgeRouterPolicies boltz.EntitySetSymbol
}

func (store *edgeRouterStoreImpl) NewStoreEntity() boltz.Entity {
	return &EdgeRouter{}
}

func (store *edgeRouterStoreImpl) GetRoleAttributesIndex() boltz.SetReadIndex {
	return store.indexRoleAttributes
}

func (store *edgeRouterStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()

	store.indexName = store.addUniqueNameField()
	store.indexRoleAttributes = store.addRoleAttributesField()

	store.AddSymbol(FieldEdgeRouterFingerprint, ast.NodeTypeString)
	store.AddSymbol(FieldEdgeRouterIsVerified, ast.NodeTypeBool)
	store.AddSymbol(FieldEdgeRouterEnrollmentToken, ast.NodeTypeString)
	store.AddSymbol(FieldEdgeRouterEnrollmentCreatedAt, ast.NodeTypeString)
	store.AddSymbol(FieldEdgeRouterEnrollmentExpiresAt, ast.NodeTypeString)
	store.symbolEdgeRouterPolicies = store.AddFkSetSymbol(EntityTypeEdgeRouterPolicies, store.stores.edgeRouterPolicy)
	store.symbolServiceEdgeRouterPolicies = store.AddFkSetSymbol(EntityTypeServiceEdgeRouterPolicies, store.stores.serviceEdgeRouterPolicy)

	store.indexRoleAttributes.AddListener(store.rolesChanged)
}

func (store *edgeRouterStoreImpl) rolesChanged(tx *bbolt.Tx, rowId []byte, _ []boltz.FieldTypeAndValue, new []boltz.FieldTypeAndValue, holder errorz.ErrorHolder) {
	// Recalculate edge router policy links
	rolesSymbol := store.stores.edgeRouterPolicy.symbolEdgeRouterRoles
	linkCollection := store.stores.edgeRouterPolicy.edgeRouterCollection
	semanticSymbol := store.stores.edgeRouterPolicy.symbolSemantic
	UpdateRelatedRoles(tx, string(rowId), rolesSymbol, linkCollection, new, holder, semanticSymbol)

	// Recalculate service edge router policy links
	rolesSymbol = store.stores.serviceEdgeRouterPolicy.symbolEdgeRouterRoles
	linkCollection = store.stores.serviceEdgeRouterPolicy.edgeRouterCollection
	semanticSymbol = store.stores.serviceEdgeRouterPolicy.symbolSemantic
	UpdateRelatedRoles(tx, string(rowId), rolesSymbol, linkCollection, new, holder, semanticSymbol)
}

func (store *edgeRouterStoreImpl) initializeLinked() {
	store.AddLinkCollection(store.symbolEdgeRouterPolicies, store.stores.edgeRouterPolicy.symbolEdgeRouters)
	store.AddLinkCollection(store.symbolServiceEdgeRouterPolicies, store.stores.serviceEdgeRouterPolicy.symbolEdgeRouters)
}

func (store *edgeRouterStoreImpl) GetNameIndex() boltz.ReadIndex {
	return store.indexName
}

func (store *edgeRouterStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*EdgeRouter, error) {
	entity := &EdgeRouter{}
	if err := store.baseLoadOneById(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *edgeRouterStoreImpl) LoadOneByName(tx *bbolt.Tx, name string) (*EdgeRouter, error) {
	id := store.indexName.Read(tx, []byte(name))
	if id != nil {
		return store.LoadOneById(tx, string(id))
	}
	return nil, nil
}

func (store *edgeRouterStoreImpl) DeleteById(ctx boltz.MutateContext, id string) error {
	if entity, _ := store.LoadOneById(ctx.Tx(), id); entity != nil {
		// Remove entity from EdgeRouterRoles in edge router policies
		if err := store.deleteEntityReferences(ctx.Tx(), entity, store.stores.edgeRouterPolicy.symbolEdgeRouterRoles); err != nil {
			return err
		}
		// Remove entity from EdgeRouterRoles in service edge router policies
		if err := store.deleteEntityReferences(ctx.Tx(), entity, store.stores.serviceEdgeRouterPolicy.symbolEdgeRouterRoles); err != nil {
			return err
		}
	}

	if store.stores.Router.IsEntityPresent(ctx.Tx(), id) {
		return store.stores.Router.DeleteById(ctx, id)
	}

	return store.baseStore.DeleteById(ctx, id)
}

func (store *edgeRouterStoreImpl) GetRoleAttributesCursorProvider(values []string, semantic string) (ast.SetCursorProvider, error) {
	return store.getRoleAttributesCursorProvider(store.indexRoleAttributes, values, semantic)
}
