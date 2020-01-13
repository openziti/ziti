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
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"time"
)

const (
	EntityTypeApiSessions        = "apiSessions"
	EntityTypeAppwans            = "appwans"
	EntityTypeCas                = "cas"
	EntityTypeClusters           = "clusters"
	EntityTypeConfigs            = "configs"
	EntityTypeEdgeRouters        = "edgeRouters"
	EntityTypeEdgeRouterPolicies = "edgeRouterPolicies"
	EntityTypeEventLogs          = "eventLogs"
	EntityTypeGeoRegions         = "geoRegions"
	EntityTypeIdentities         = "identities"
	EntityTypeIdentityTypes      = "identityTypes"
	EntityTypeServices           = "services"
	EntityTypeServicePolicies    = "servicePolicies"
	EntityTypeSessions           = "sessions"
	EntityTypeSessionCerts       = "sessionCerts"
	EntityTypeEnrollments        = "enrollments"
	EntityTypeAuthenticators     = "authenticators"
	EdgeBucket                   = "edge"

	FieldId             = "id"
	FieldName           = "name"
	FieldRoleAttributes = "roleAttributes"
	FieldCreatedAt      = "createdAt"
	FieldUpdatedAt      = "updatedAt"
	FieldTags           = "tags"
)

type BaseEdgeEntity interface {
	boltz.BaseEntity
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
	GetTags() map[string]interface{}

	setCreateAt(createdAt time.Time)
	setUpdatedAt(updatedAt time.Time)
	setTags(tags map[string]interface{})
}

func NewBaseEdgeEntity(id string, tags map[string]interface{}) *BaseEdgeEntityImpl {
	return &BaseEdgeEntityImpl{
		Id: id,
		EdgeEntityFields: EdgeEntityFields{
			Tags: tags,
		},
	}
}

type BaseEdgeEntityImpl struct {
	Id string
	EdgeEntityFields
}

type EdgeEntityFields struct {
	CreatedAt time.Time
	UpdatedAt time.Time
	Tags      map[string]interface{}
	Migrate   bool
}

func (entity *BaseEdgeEntityImpl) GetId() string {
	return entity.Id
}

func (entity *BaseEdgeEntityImpl) SetId(id string) {
	entity.Id = id
}

func (entity *EdgeEntityFields) GetCreatedAt() time.Time {
	return entity.CreatedAt
}

func (entity *EdgeEntityFields) GetUpdatedAt() time.Time {
	return entity.UpdatedAt
}

func (entity *EdgeEntityFields) GetTags() map[string]interface{} {
	return entity.Tags
}

func (entity *EdgeEntityFields) setCreateAt(createdAt time.Time) {
	entity.CreatedAt = createdAt
}

func (entity *EdgeEntityFields) setUpdatedAt(updatedAt time.Time) {
	entity.UpdatedAt = updatedAt
}

func (entity *EdgeEntityFields) setTags(tags map[string]interface{}) {
	entity.Tags = tags
}

func (entity *EdgeEntityFields) LoadBaseValues(bucket *boltz.TypedBucket) {
	entity.CreatedAt = bucket.GetTimeOrError("createdAt")
	entity.UpdatedAt = bucket.GetTimeOrError("updatedAt")
	entity.Tags = bucket.GetMap("tags")
}

func (entity *EdgeEntityFields) SetBaseValues(ctx *boltz.PersistContext) {
	if ctx.IsCreate {
		entity.CreateBaseValues(ctx.Bucket)
	} else {
		entity.UpdateBaseValues(ctx.Bucket, ctx.FieldChecker)
	}
}

func (entity *EdgeEntityFields) CreateBaseValues(bucket *boltz.TypedBucket) {
	now := time.Now()
	if entity.Migrate {
		bucket.SetTimeP(FieldCreatedAt, &entity.CreatedAt, nil)
		bucket.SetTimeP(FieldUpdatedAt, &entity.UpdatedAt, nil)
	} else {
		bucket.SetTimeP(FieldCreatedAt, &now, nil)
		bucket.SetTimeP(FieldUpdatedAt, &now, nil)
	}
	bucket.PutMap(FieldTags, entity.Tags, nil)
}

func (entity *EdgeEntityFields) UpdateBaseValues(bucket *boltz.TypedBucket, fieldChecker boltz.FieldChecker) {
	now := time.Now()
	bucket.SetTimeP(FieldUpdatedAt, &now, nil)
	bucket.PutMap(FieldTags, entity.Tags, fieldChecker)
}

func toStringStringMap(m map[string]interface{}) map[string]string {
	result := map[string]string{}
	for k, v := range m {
		result[k] = v.(string)
	}
	return result
}

func toStringInterfaceMap(m map[string]string) map[string]interface{} {
	result := map[string]interface{}{}
	for k, v := range m {
		result[k] = v
	}
	return result
}
