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

package model

import (
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/fabric/controller/apierror"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/openziti/foundation/util/errorz"
	"github.com/pkg/errors"
	"github.com/xeipuuv/gojsonschema"
	"go.etcd.io/bbolt"
	"reflect"
)

type Config struct {
	models.BaseEntity
	Name   string
	TypeId string
	Data   map[string]interface{}
}

func (entity *Config) toBoltEntity(tx *bbolt.Tx, handler Handler) (boltz.Entity, error) {
	if entity.TypeId != "" {
		providedType := entity.TypeId
		configTypeStore := handler.GetEnv().GetStores().ConfigType
		if !configTypeStore.IsEntityPresent(tx, entity.TypeId) {
			return nil, errorz.NewFieldError("invalid config type", persistence.FieldConfigType, providedType)
		}
	}

	if entity.TypeId == "" {
		currentConfig, err := handler.GetEnv().GetHandlers().Config.readInTx(tx, entity.Id)
		if err != nil {
			return nil, err
		}
		entity.TypeId = currentConfig.TypeId
	}

	if configType, _ := handler.GetEnv().GetHandlers().ConfigType.readInTx(tx, entity.TypeId); configType != nil && len(configType.Schema) > 0 {
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

	return &persistence.Config{
		BaseExtEntity: *boltz.NewExtEntity(entity.Id, entity.Tags),
		Name:          entity.Name,
		Type:          entity.TypeId,
		Data:          entity.Data,
	}, nil
}

func (entity *Config) toBoltEntityForCreate(tx *bbolt.Tx, handler Handler) (boltz.Entity, error) {
	if entity.TypeId == "" {
		return nil, errorz.NewFieldError("config type must be specified", persistence.FieldConfigType, entity.TypeId)
	}
	return entity.toBoltEntity(tx, handler)
}

func (entity *Config) toBoltEntityForUpdate(tx *bbolt.Tx, handler Handler) (boltz.Entity, error) {
	return entity.toBoltEntity(tx, handler)
}

func (entity *Config) toBoltEntityForPatch(tx *bbolt.Tx, handler Handler, checker boltz.FieldChecker) (boltz.Entity, error) {
	return entity.toBoltEntity(tx, handler)
}

func (entity *Config) fillFrom(_ Handler, _ *bbolt.Tx, boltEntity boltz.Entity) error {
	boltConfig, ok := boltEntity.(*persistence.Config)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model config", reflect.TypeOf(boltEntity))
	}

	entity.FillCommon(boltConfig)
	entity.Name = boltConfig.Name
	entity.TypeId = boltConfig.Type
	entity.Data = boltConfig.Data
	return nil
}
