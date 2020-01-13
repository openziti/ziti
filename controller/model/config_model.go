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
	"github.com/netfoundry/ziti-edge/controller/persistence"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
)

type Config struct {
	BaseModelEntityImpl
	Name string
	Data map[string]interface{}
}

func (entity *Config) ToBoltEntityForCreate(_ *bbolt.Tx, _ Handler) (persistence.BaseEdgeEntity, error) {
	return &persistence.Config{
		BaseEdgeEntityImpl: *persistence.NewBaseEdgeEntity(entity.Id, entity.Tags),
		Name:               entity.Name,
		Data:               entity.Data,
	}, nil
}

func (entity *Config) ToBoltEntityForUpdate(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	return entity.ToBoltEntityForCreate(tx, handler)
}

func (entity *Config) ToBoltEntityForPatch(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	return entity.ToBoltEntityForCreate(tx, handler)
}

func (entity *Config) FillFrom(_ Handler, _ *bbolt.Tx, boltEntity boltz.BaseEntity) error {
	boltConfig, ok := boltEntity.(*persistence.Config)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model config", reflect.TypeOf(boltEntity))
	}

	entity.fillCommon(boltConfig)
	entity.Name = boltConfig.Name
	entity.Data = boltConfig.Data
	return nil
}
