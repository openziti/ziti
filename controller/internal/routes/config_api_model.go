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

package routes

import (
	"fmt"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/response"
	"github.com/openziti/ziti/controller/models"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/foundation/v2/stringz"
	"math"
)

const EntityNameConfig = "configs"

var ConfigLinkFactory = NewBasicLinkFactory(EntityNameConfig)

func MapCreateConfigToModel(config *rest_model.ConfigCreate) (*model.Config, error) {
	ret := &model.Config{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(config.Tags),
		},
		Name:   stringz.OrEmpty(config.Name),
		TypeId: stringz.OrEmpty(config.ConfigTypeID),
	}

	if config.Data != nil {
		if dataMap, ok := config.Data.(map[string]interface{}); ok {
			ret.Data = dataMap
			narrowJsonTypesMap(ret.Data)
		} else {
			return nil, errorz.NewFieldError("invalid type, expected object", "data", config.Data)
		}
	}

	return ret, nil
}

func MapUpdateConfigToModel(id string, config *rest_model.ConfigUpdate) (*model.Config, error) {
	ret := &model.Config{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(config.Tags),
			Id:   id,
		},
		Name: stringz.OrEmpty(config.Name),
	}

	if config.Data != nil {
		if dataMap, ok := config.Data.(map[string]interface{}); ok {
			ret.Data = dataMap
			narrowJsonTypesMap(ret.Data)
		} else {
			return nil, errorz.NewFieldError("invalid type, expected object", "data", config.Data)
		}
	}

	return ret, nil
}

func MapPatchConfigToModel(id string, config *rest_model.ConfigPatch) (*model.Config, error) {
	ret := &model.Config{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(config.Tags),
			Id:   id,
		},
		Name: config.Name,
	}

	if config.Data != nil {
		if dataMap, ok := config.Data.(map[string]interface{}); ok {
			ret.Data = dataMap
			narrowJsonTypesMap(ret.Data)
		} else {
			return nil, errorz.NewFieldError("invalid type, expected object", "data", config.Data)
		}
	}

	narrowJsonTypesMap(ret.Data)

	return ret, nil
}

func MapConfigToRestEntity(ae *env.AppEnv, _ *response.RequestContext, config *model.Config) (interface{}, error) {
	return MapConfigToRestModel(ae, config)
}

func MapConfigToRestModel(ae *env.AppEnv, config *model.Config) (*rest_model.ConfigDetail, error) {

	configType, err := ae.Managers.ConfigType.Read(config.TypeId)

	if err != nil {
		return nil, fmt.Errorf("could not find type [%s]: %v", config.TypeId, err)
	}

	ret := &rest_model.ConfigDetail{
		BaseEntity:   BaseEntityToRestModel(config, ConfigLinkFactory),
		Data:         config.Data,
		Name:         &config.Name,
		ConfigType:   ToEntityRef(configType.Name, configType, ConfigTypeLinkFactory),
		ConfigTypeID: &config.TypeId,
	}

	return ret, nil
}

func resolveParsedNumber(v interface{}) interface{} {
	if parsedNumber, ok := v.(ParsedNumber); ok {
		//floats don't parse as int, try int first, then float, else give up
		if intVal, err := parsedNumber.Int64(); err == nil {
			v = intVal
		} else if floatVal, err := parsedNumber.Float64(); err == nil {
			v = floatVal
			intVal := math.Trunc(floatVal)
			if intVal == floatVal {
				v = intVal
			}
		}
	}
	return v
}

func narrowJsonTypesList(l []interface{}) {
	for i, v := range l {
		v = resolveParsedNumber(v)

		switch val := v.(type) {
		case []interface{}:
			narrowJsonTypesList(val)
		case map[string]interface{}:
			narrowJsonTypesMap(val)
		default:
			l[i] = v
		}
	}
}

func narrowJsonTypesMap(m map[string]interface{}) {
	for k, v := range m {
		v = resolveParsedNumber(v)

		switch val := v.(type) {
		case []interface{}:
			narrowJsonTypesList(val)
		case map[string]interface{}:
			narrowJsonTypesMap(val)
		default:
			m[k] = v
		}
	}
}

type ParsedNumber interface {
	String() string
	Float64() (float64, error)
	Int64() (int64, error)
}
