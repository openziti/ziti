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

package model

import (
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/controller/apierror"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/models"
	"github.com/xeipuuv/gojsonschema"
	"go.etcd.io/bbolt"
)

type Config struct {
	models.BaseEntity
	Name   string
	TypeId string
	Data   map[string]interface{}
}

func (entity *Config) toBoltEntity(tx *bbolt.Tx, env Env) (*db.Config, error) {
	if entity.TypeId != "" {
		providedType := entity.TypeId
		configTypeStore := env.GetStores().ConfigType
		if !configTypeStore.IsEntityPresent(tx, entity.TypeId) {
			return nil, errorz.NewFieldError("invalid config type", db.FieldConfigType, providedType)
		}
	}

	if entity.TypeId == "" {
		currentConfig, err := env.GetManagers().Config.readInTx(tx, entity.Id)
		if err != nil {
			return nil, err
		}
		entity.TypeId = currentConfig.TypeId
	}

	if configType, _ := env.GetManagers().ConfigType.readInTx(tx, entity.TypeId); configType != nil && len(configType.Schema) > 0 {
		compileSchema, err := configType.GetCompiledSchema()
		if err != nil {
			return nil, err
		}
		jsonLoader := gojsonschema.NewGoLoader(entity.Data)
		result, err := compileSchema.Validate(jsonLoader)
		if err != nil {
			return nil, err
		}
		if !result.Valid() {
			return nil, apierror.NewValidationErrors(result)
		}
	}

	return &db.Config{
		BaseExtEntity: *boltz.NewExtEntity(entity.Id, entity.Tags),
		Name:          entity.Name,
		Type:          entity.TypeId,
		Data:          entity.Data,
	}, nil
}

func (entity *Config) toBoltEntityForCreate(tx *bbolt.Tx, env Env) (*db.Config, error) {
	if entity.TypeId == "" {
		return nil, errorz.NewFieldError("config type must be specified", db.FieldConfigType, entity.TypeId)
	}
	return entity.toBoltEntity(tx, env)
}

func (entity *Config) toBoltEntityForUpdate(tx *bbolt.Tx, env Env, _ boltz.FieldChecker) (*db.Config, error) {
	return entity.toBoltEntity(tx, env)
}

func (entity *Config) fillFrom(_ Env, _ *bbolt.Tx, boltConfig *db.Config) error {
	entity.FillCommon(boltConfig)
	entity.Name = boltConfig.Name
	entity.TypeId = boltConfig.Type
	entity.Data = boltConfig.Data
	return nil
}
