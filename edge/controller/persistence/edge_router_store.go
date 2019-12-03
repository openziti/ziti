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
	"github.com/netfoundry/ziti-foundation/storage/ast"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
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

type EdgeRouter struct {
	BaseEdgeEntityImpl
	Name                string
	ClusterId           string
	IsVerified          bool
	Fingerprint         *string
	CertPem             *string
	EnrollmentToken     *string
	Hostname            *string
	EnrollmentJwt       *string
	EnrollmentCreatedAt *time.Time
	EnrollmentExpiresAt *time.Time
	EdgeRouterProtocols map[string]string
}

var edgeRouterFieldMappings = map[string]string{FieldEdgeRouterCluster: "clusterId"}

func (entity *EdgeRouter) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
	entity.Fingerprint = bucket.GetString(FieldEdgeRouterFingerprint)
	entity.CertPem = bucket.GetString(FieldEdgeRouterCertPEM)
	entity.ClusterId = bucket.GetStringOrError(FieldEdgeRouterCluster)
	entity.IsVerified = bucket.GetBoolWithDefault(FieldEdgeRouterIsVerified, false)

	entity.EnrollmentToken = bucket.GetString(FieldEdgeRouterEnrollmentToken)
	entity.Hostname = bucket.GetString(FieldEdgeRouterHostname)
	entity.EnrollmentJwt = bucket.GetString(FieldEdgeRouterEnrollmentJwt)
	entity.EnrollmentCreatedAt = bucket.GetTime(FieldEdgeRouterEnrollmentCreatedAt)
	entity.EnrollmentExpiresAt = bucket.GetTime(FieldEdgeRouterEnrollmentExpiresAt)
	entity.EdgeRouterProtocols = toStringStringMap(bucket.GetMap(FieldEdgeRouterProtocols))
}

func (entity *EdgeRouter) SetValues(ctx *boltz.PersistContext) {
	ctx.WithFieldOverrides(edgeRouterFieldMappings)

	entity.SetBaseValues(ctx)
	ctx.SetString(FieldName, entity.Name)
	ctx.SetStringP(FieldEdgeRouterFingerprint, entity.Fingerprint)
	ctx.SetStringP(FieldEdgeRouterCertPEM, entity.CertPem)
	ctx.SetString(FieldEdgeRouterCluster, entity.ClusterId)
	ctx.SetBool(FieldEdgeRouterIsVerified, entity.IsVerified)
	ctx.SetStringP(FieldEdgeRouterEnrollmentToken, entity.EnrollmentToken)
	ctx.SetStringP(FieldEdgeRouterHostname, entity.Hostname)
	ctx.SetStringP(FieldEdgeRouterEnrollmentJwt, entity.EnrollmentJwt)
	ctx.SetTimeP(FieldEdgeRouterEnrollmentCreatedAt, entity.EnrollmentCreatedAt)
	ctx.SetTimeP(FieldEdgeRouterEnrollmentExpiresAt, entity.EnrollmentExpiresAt)
	ctx.SetMap(FieldEdgeRouterProtocols, toStringInterfaceMap(entity.EdgeRouterProtocols))
}

func (entity *EdgeRouter) GetEntityType() string {
	return EntityTypeEdgeRouters
}

type EdgeRouterStore interface {
	Store
	LoadOneById(tx *bbolt.Tx, id string) (*EdgeRouter, error)
	LoadOneByName(tx *bbolt.Tx, id string) (*EdgeRouter, error)
	LoadOneByQuery(tx *bbolt.Tx, query string) (*EdgeRouter, error)
	UpdateCluster(ctx boltz.MutateContext, id string, clusterId string) error
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

	indexName       boltz.ReadIndex
	symbolClusterId boltz.EntitySymbol
}

func (store *edgeRouterStoreImpl) NewStoreEntity() boltz.BaseEntity {
	return &EdgeRouter{}
}

func (store *edgeRouterStoreImpl) initializeLocal() {
	store.addBaseFields()

	store.indexName = store.addUniqueNameField()
	store.symbolClusterId = store.AddFkSymbol(FieldEdgeRouterCluster, store.stores.cluster)

	store.AddSymbol(FieldEdgeRouterFingerprint, ast.NodeTypeString)
	store.AddSymbol(FieldEdgeRouterIsVerified, ast.NodeTypeBool)
	store.AddSymbol(FieldEdgeRouterEnrollmentToken, ast.NodeTypeString)
	store.AddSymbol(FieldEdgeRouterEnrollmentCreatedAt, ast.NodeTypeString)
	store.AddSymbol(FieldEdgeRouterEnrollmentExpiresAt, ast.NodeTypeString)
}

func (store *edgeRouterStoreImpl) initializeLinked() {
	store.AddFkIndex(store.symbolClusterId, store.stores.cluster.symbolEdgeRouters)
}

func (store *edgeRouterStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*EdgeRouter, error) {
	entity := &EdgeRouter{}
	if found, err := store.BaseLoadOneById(tx, id, entity); !found || err != nil {
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

func (store *edgeRouterStoreImpl) LoadOneByQuery(tx *bbolt.Tx, query string) (*EdgeRouter, error) {
	entity := &EdgeRouter{}
	if found, err := store.BaseLoadOneByQuery(tx, query, entity); !found || err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *edgeRouterStoreImpl) UpdateCluster(ctx boltz.MutateContext, id string, clusterId string) error {
	entity, err := store.LoadOneById(ctx.Tx(), id)
	if err != nil {
		return err
	}
	entity.ClusterId = clusterId
	return store.Update(ctx, entity, clusterIdFieldChecker{})
}

type clusterIdFieldChecker struct{}

func (checker clusterIdFieldChecker) IsUpdated(field string) bool {
	return "clusterId" == field
}
