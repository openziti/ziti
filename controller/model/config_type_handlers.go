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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/util/stringz"
	"github.com/openziti/storage/boltz"
	"go.etcd.io/bbolt"
)

const (
	ConfigTypeAll = "all"
)

func NewConfigTypeHandler(env Env) *ConfigTypeHandler {
	handler := &ConfigTypeHandler{
		baseEntityManager: newBaseEntityManager(env, env.GetStores().ConfigType),
	}
	handler.impl = handler
	return handler
}

type ConfigTypeHandler struct {
	baseEntityManager
}

func (handler *ConfigTypeHandler) newModelEntity() boltEntitySink {
	return &ConfigType{}
}

func (handler *ConfigTypeHandler) Create(configType *ConfigType) (string, error) {
	return handler.createEntity(configType)
}

func (handler *ConfigTypeHandler) Read(id string) (*ConfigType, error) {
	modelEntity := &ConfigType{}
	if err := handler.readEntity(id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *ConfigTypeHandler) readInTx(tx *bbolt.Tx, id string) (*ConfigType, error) {
	modelEntity := &ConfigType{}
	if err := handler.readEntityInTx(tx, id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *ConfigTypeHandler) ReadByName(name string) (*ConfigType, error) {
	modelEntity := &ConfigType{}
	nameIndex := handler.env.GetStores().ConfigType.GetNameIndex()
	if err := handler.readEntityWithIndex("name", []byte(name), nameIndex, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *ConfigTypeHandler) Update(configType *ConfigType) error {
	return handler.updateEntity(configType, nil)
}

func (handler *ConfigTypeHandler) Patch(configType *ConfigType, checker boltz.FieldChecker) error {
	return handler.patchEntity(configType, checker)
}

func (handler *ConfigTypeHandler) Delete(id string) error {
	return handler.deleteEntity(id)
}

func (handler *ConfigTypeHandler) MapConfigTypeNamesToIds(values []string, identityId string) map[string]struct{} {
	var result []string
	if stringz.Contains(values, "all") {
		result = []string{"all"}
	} else {
		for _, val := range values {
			if configType, _ := handler.Read(val); configType != nil {
				result = append(result, val)
			} else if configType, _ := handler.ReadByName(val); configType != nil {
				result = append(result, configType.Id)
			} else {
				pfxlog.Logger().Debugf("user %v submitted %v as a config type of interest, but no matching records found", identityId, val)
			}
		}
	}
	return stringz.SliceToSet(result)
}
