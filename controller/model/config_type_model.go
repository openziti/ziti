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
	"fmt"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/util/errorz"
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
	"github.com/xeipuuv/gojsonschema"
	"go.etcd.io/bbolt"
	"reflect"
)

type ConfigType struct {
	models.BaseEntity
	Name   string
	Schema map[string]interface{}
}

func (entity *ConfigType) GetCompiledSchema() (*gojsonschema.Schema, error) {
	if len(entity.Schema) == 0 {
		return nil, errors.Errorf("no schema defined on config type %v", entity.Name)
	}
	entitySchemaLoader := gojsonschema.NewGoLoader(entity.Schema)
	schemaLoader := gojsonschema.NewSchemaLoader()
	return schemaLoader.Compile(entitySchemaLoader)
}

func (entity *ConfigType) toBoltEntity() (boltz.Entity, error) {
	if entity.Name == ConfigTypeAll {
		return nil, errorz.NewFieldError(fmt.Sprintf("%v is a keyword and may not be used as a config type name", entity.Name), "name", entity.Name)
	}

	if len(entity.Schema) > 0 {
		if _, err := entity.GetCompiledSchema(); err != nil {
			return nil, errorz.NewFieldError(fmt.Sprintf("invalid schema %v", err), "schema", entity.Schema)
		}
	}
	return &persistence.ConfigType{
		BaseExtEntity: *boltz.NewExtEntity(entity.Id, entity.Tags),
		Name:          entity.Name,
		Schema:        entity.Schema,
	}, nil
}

func (entity *ConfigType) toBoltEntityForCreate(*bbolt.Tx, EntityManager) (boltz.Entity, error) {
	return entity.toBoltEntity()
}

func (entity *ConfigType) toBoltEntityForUpdate(*bbolt.Tx, EntityManager) (boltz.Entity, error) {
	return entity.toBoltEntity()
}

func (entity *ConfigType) toBoltEntityForPatch(*bbolt.Tx, EntityManager, boltz.FieldChecker) (boltz.Entity, error) {
	return entity.toBoltEntity()
}

func (entity *ConfigType) fillFrom(_ EntityManager, _ *bbolt.Tx, boltEntity boltz.Entity) error {
	boltConfigType, ok := boltEntity.(*persistence.ConfigType)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model configType", reflect.TypeOf(boltEntity))
	}

	entity.FillCommon(boltConfigType)
	entity.Name = boltConfigType.Name
	entity.Schema = boltConfigType.Schema
	return nil
}
