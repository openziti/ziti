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

package migration

import (
	"github.com/jinzhu/gorm"
	"time"
)

type BaseDbEntity struct {
	ID        string `gorm:"primary_key"`
	CreatedAt *time.Time
	UpdatedAt *time.Time
	Tags      *PropertyMap
}

func NewBaseDbEntity() BaseDbEntity {
	return BaseDbEntity{
		Tags: &PropertyMap{},
		ID:   NewId(),
	}
}

func (b *BaseDbEntity) GetId() string {
	return b.ID
}

func (b *BaseDbEntity) SetId(id string) {
	b.ID = id
}

func (b *BaseDbEntity) SetUpdatedAt(t *time.Time) {
	b.UpdatedAt = t
}

func (b *BaseDbEntity) SetTags(tags *PropertyMap) {
	b.Tags = tags
}

func (b *BaseDbEntity) GetTags() *PropertyMap {
	return b.Tags
}

func (b *BaseDbEntity) ClearAssociations(typeEntity interface{}, tx *gorm.DB, columns ...string) error {
	for _, column := range columns {
		res := tx.Model(typeEntity).Association(column).Clear()
		if res.Error != nil {
			return res.Error
		}
	}
	return nil
}
