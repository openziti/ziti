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
	"github.com/openziti/storage/boltz"
)

func NewIdentityTypeHandler(env Env) *IdentityTypeHandler {
	handler := &IdentityTypeHandler{
		baseEntityManager: newBaseEntityManager(env, env.GetStores().IdentityType),
	}
	handler.impl = handler
	return handler
}

type IdentityTypeHandler struct {
	baseEntityManager
}

func (handler *IdentityTypeHandler) newModelEntity() boltEntitySink {
	return &IdentityType{}
}

func (handler *IdentityTypeHandler) Create(IdentityTypeModel *IdentityType) (string, error) {
	return handler.createEntity(IdentityTypeModel)
}

func (handler *IdentityTypeHandler) Read(id string) (*IdentityType, error) {
	modelEntity := &IdentityType{}
	if err := handler.readEntity(id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *IdentityTypeHandler) ReadByIdOrName(idOrName string) (*IdentityType, error) {
	modelEntity := &IdentityType{}
	err := handler.readEntity(idOrName, modelEntity)

	if err == nil {
		return modelEntity, nil
	}

	if !boltz.IsErrNotFoundErr(err) {
		return nil, err
	}

	if modelEntity.Id == "" {
		modelEntity, err = handler.ReadByName(idOrName)
	}

	if err != nil {
		return nil, err
	}

	return modelEntity, nil
}

func (handler *IdentityTypeHandler) Delete(id string) error {
	return handler.deleteEntity(id)
}

func (handler *IdentityTypeHandler) ReadByName(name string) (*IdentityType, error) {
	modelIdentityType := &IdentityType{}
	nameIndex := handler.env.GetStores().IdentityType.GetNameIndex()
	if err := handler.readEntityWithIndex("name", []byte(name), nameIndex, modelIdentityType); err != nil {
		return nil, err
	}
	return modelIdentityType, nil
}
