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
	"github.com/openziti/foundation/storage/boltz"
)

func NewPostureCheckTypeHandler(env Env) *PostureCheckTypeHandler {
	handler := &PostureCheckTypeHandler{
		baseHandler: newBaseHandler(env, env.GetStores().PostureCheckType),
	}
	handler.impl = handler
	return handler
}

type PostureCheckTypeHandler struct {
	baseHandler
}

func (handler *PostureCheckTypeHandler) newModelEntity() boltEntitySink {
	return &PostureCheckType{}
}

func (handler *PostureCheckTypeHandler) Create(PostureCheckTypeModel *PostureCheckType) (string, error) {
	return handler.createEntity(PostureCheckTypeModel)
}

func (handler *PostureCheckTypeHandler) Read(id string) (*PostureCheckType, error) {
	modelEntity := &PostureCheckType{}
	if err := handler.readEntity(id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *PostureCheckTypeHandler) ReadByIdOrName(idOrName string) (*PostureCheckType, error) {
	modelEntity := &PostureCheckType{}
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

func (handler *PostureCheckTypeHandler) Delete(id string) error {
	return handler.deleteEntity(id)
}

func (handler *PostureCheckTypeHandler) ReadByName(name string) (*PostureCheckType, error) {
	modelPostureCheckType := &PostureCheckType{}
	nameIndex := handler.env.GetStores().PostureCheckType.GetNameIndex()
	if err := handler.readEntityWithIndex("name", []byte(name), nameIndex, modelPostureCheckType); err != nil {
		return nil, err
	}
	return modelPostureCheckType, nil
}
