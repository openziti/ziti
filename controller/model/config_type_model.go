/*
	Copyright 2020 Netfoundry, Inc.

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
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
)

type ConfigType struct {
	BaseModelEntityImpl
	Name string
	Type string
	Data map[string]interface{}
}

func (entity *ConfigType) ToBoltEntityForCreate(_ *bbolt.Tx, _ Handler) (persistence.BaseEdgeEntity, error) {
	if entity.Name == ConfigTypeAll {
		return nil, validation.NewFieldError(fmt.Sprintf("%v is a keyword and may not be used as a config type name", entity.Name), "name", entity.Name)
	}
	return &persistence.ConfigType{
		BaseEdgeEntityImpl: *persistence.NewBaseEdgeEntity(entity.Id, entity.Tags),
		Name:               entity.Name,
	}, nil
}

func (entity *ConfigType) ToBoltEntityForUpdate(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	return entity.ToBoltEntityForCreate(tx, handler)
}

func (entity *ConfigType) ToBoltEntityForPatch(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	return entity.ToBoltEntityForCreate(tx, handler)
}

func (entity *ConfigType) FillFrom(_ Handler, _ *bbolt.Tx, boltEntity boltz.BaseEntity) error {
	boltConfigType, ok := boltEntity.(*persistence.ConfigType)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model configType", reflect.TypeOf(boltEntity))
	}

	entity.fillCommon(boltConfigType)
	entity.Name = boltConfigType.Name
	return nil
}
