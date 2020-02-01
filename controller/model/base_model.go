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

package model

import (
	"fmt"
	"github.com/netfoundry/ziti-edge/controller/persistence"
	"github.com/netfoundry/ziti-edge/controller/validation"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/netfoundry/ziti-foundation/util/stringz"
	"go.etcd.io/bbolt"
	"time"
)

type BaseModelEntity interface {
	GetId() string
	setId(string)
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
	GetTags() map[string]interface{}
	FillFrom(handler Handler, tx *bbolt.Tx, boltEntity boltz.BaseEntity) error

	ToBoltEntityForCreate(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error)
	ToBoltEntityForUpdate(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error)
	ToBoltEntityForPatch(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error)
}

type BaseModelEntityImpl struct {
	Id        string
	CreatedAt time.Time
	UpdatedAt time.Time
	Tags      map[string]interface{}
}

func (entity *BaseModelEntityImpl) GetId() string {
	return entity.Id
}

func (entity *BaseModelEntityImpl) setId(id string) {
	entity.Id = id
}

func (entity *BaseModelEntityImpl) GetCreatedAt() time.Time {
	return entity.CreatedAt
}

func (entity *BaseModelEntityImpl) GetUpdatedAt() time.Time {
	return entity.UpdatedAt
}

func (entity *BaseModelEntityImpl) GetTags() map[string]interface{} {
	return entity.Tags
}

func (entity *BaseModelEntityImpl) fillCommon(boltEntity persistence.BaseEdgeEntity) {
	entity.Id = boltEntity.GetId()
	entity.CreatedAt = boltEntity.GetCreatedAt()
	entity.UpdatedAt = boltEntity.GetUpdatedAt()
	entity.Tags = boltEntity.GetTags()
}

type QueryMetaData struct {
	Count            int64
	Limit            int64
	Offset           int64
	FilterableFields []string
}

func ValidateEntityList(tx *bbolt.Tx, store boltz.ListStore, field string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	query := persistence.ToInFilter(ids...) + " limit none"
	foundIds, _, err := store.QueryIds(tx, query)

	if err != nil {
		return err
	}

	if len(ids) != len(foundIds) {
		invalidIds := stringz.Difference(ids, foundIds)
		return validation.NewFieldError(fmt.Sprintf("%v(s) not found", store.GetEntityType()), field, invalidIds)
	}
	return nil
}
