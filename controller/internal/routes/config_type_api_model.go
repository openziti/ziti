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

package routes

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/util/stringz"
)

const EntityNameConfigType = "config-types"

var ConfigTypeLinkFactory = NewBasicLinkFactory(EntityNameConfigType)

func MapCreateConfigTypeToModel(configType *rest_model.ConfigTypeCreate) *model.ConfigType {
	ret := &model.ConfigType{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(configType.Tags),
		},
		Name: stringz.OrEmpty(configType.Name),
	}

	if schemaMap, ok := configType.Schema.(map[string]interface{}); ok {
		ret.Schema = schemaMap
	}

	return ret
}

func MapUpdateConfigTypeToModel(id string, configType *rest_model.ConfigTypeUpdate) *model.ConfigType {
	ret := &model.ConfigType{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(configType.Tags),
			Id:   id,
		},
		Name: stringz.OrEmpty(configType.Name),
	}

	if schemaMap, ok := configType.Schema.(map[string]interface{}); ok {
		ret.Schema = schemaMap
	}

	return ret
}

func MapPatchConfigTypeToModel(id string, configType *rest_model.ConfigTypePatch) *model.ConfigType {
	ret := &model.ConfigType{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(configType.Tags),
			Id:   id,
		},
		Name: configType.Name,
	}

	if schemaMap, ok := configType.Schema.(map[string]interface{}); ok {
		ret.Schema = schemaMap
	}

	return ret
}

func MapConfigTypeToRestEntity(_ *env.AppEnv, _ *response.RequestContext, e models.Entity) (interface{}, error) {
	configType, ok := e.(*model.ConfigType)

	if !ok {
		err := fmt.Errorf("entity is not a ConfigType \"%s\"", e.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	restModel, err := MapConfigTypeToRestModel(configType)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return restModel, nil
}

func MapConfigTypeToRestModel(configType *model.ConfigType) (*rest_model.ConfigTypeDetail, error) {
	ret := &rest_model.ConfigTypeDetail{
		BaseEntity: BaseEntityToRestModel(configType, ConfigTypeLinkFactory),
		Name:       &configType.Name,
		Schema:     configType.Schema,
	}

	return ret, nil
}
