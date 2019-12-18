/*
	Copyright 2019 Netfoundry, Inc.

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

type Appwan struct {
	BaseModelEntityImpl
	Name       string   `json:"name"`
	Identities []string `json:"identities"`
	Services   []string `json:"services"`
}

func (entity *Appwan) ToBoltEntityForCreate(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	if err := ValidateEntityList(tx, handler.GetEnv().GetStores().EdgeService, "services", entity.Services); err != nil {
		return nil, err
	}

	if err := ValidateEntityList(tx, handler.GetEnv().GetStores().Identity, "identities", entity.Identities); err != nil {
		return nil, err
	}

	appwan := &persistence.Appwan{
		Name:               entity.Name,
		BaseEdgeEntityImpl: *persistence.NewBaseEdgeEntity(entity.Id, entity.Tags),
		Identities:         entity.Identities,
		Services:           entity.Services,
	}

	return appwan, nil
}

func (entity *Appwan) ToBoltEntityForUpdate(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	return &persistence.Appwan{
		Name:               entity.Name,
		BaseEdgeEntityImpl: *persistence.NewBaseEdgeEntity(entity.Id, entity.Tags),
	}, nil
}

func (entity *Appwan) ToBoltEntityForPatch(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	return entity.ToBoltEntityForUpdate(tx, handler)
}

func (entity *Appwan) FillFrom(handler Handler, tx *bbolt.Tx, boltEntity boltz.BaseEntity) error {
	boltAppwan, ok := boltEntity.(*persistence.Appwan)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model appwan", reflect.TypeOf(boltEntity))
	}
	entity.fillCommon(boltAppwan)
	entity.Name = boltAppwan.Name
	entity.Identities = boltAppwan.Identities
	entity.Services = boltAppwan.Services
	return nil
}