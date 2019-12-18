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

package env

import (
	"github.com/netfoundry/ziti-edge/controller/model"
	"github.com/netfoundry/ziti-edge/controller/response"
	"github.com/netfoundry/ziti-edge/migration"
	"time"
)

type BaseApi struct {
	Id        string                 `json:"id"`
	CreatedAt *time.Time             `json:"createdAt"`
	UpdatedAt *time.Time             `json:"updatedAt"`
	Links     *response.Links        `json:"_links"`
	Tags      map[string]interface{} `json:"tags"`
}

func FromBaseDbEntity(entity *migration.BaseDbEntity) *BaseApi {
	var tags map[string]interface{}
	if entity.Tags != nil {
		tags = *entity.Tags
	}
	return &BaseApi{
		Id:        entity.ID,
		UpdatedAt: entity.UpdatedAt,
		CreatedAt: entity.CreatedAt,
		Tags:      tags,
	}
}

func FromBaseModelEntity(entity model.BaseModelEntity) *BaseApi {
	createdAt := entity.GetCreatedAt()
	updatedAt := entity.GetUpdatedAt()
	return &BaseApi{
		Id:        entity.GetId(),
		UpdatedAt: &updatedAt,
		CreatedAt: &createdAt,
		Tags:      entity.GetTags(),
	}
}
