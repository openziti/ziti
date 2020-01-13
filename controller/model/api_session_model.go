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
	"github.com/netfoundry/ziti-foundation/util/stringz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
)

type ApiSession struct {
	BaseModelEntityImpl
	Token       string
	IdentityId  string
	Identity    *Identity
	ConfigTypes map[string]struct{}
}

func (entity *ApiSession) ToBoltEntityForCreate(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	if !handler.GetEnv().GetStores().Identity.IsEntityPresent(tx, entity.IdentityId) {
		return nil, NewFieldError("identity not found", "IdentityId", entity.IdentityId)
	}

	boltEntity := &persistence.ApiSession{
		BaseEdgeEntityImpl: *persistence.NewBaseEdgeEntity(entity.Id, entity.Tags),
		Token:              entity.Token,
		IdentityId:         entity.IdentityId,
		ConfigTypes:        stringz.SetToSlice(entity.ConfigTypes),
	}

	return boltEntity, nil
}

func (entity *ApiSession) ToBoltEntityForUpdate(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	return entity.ToBoltEntityForCreate(tx, handler)
}

func (entity *ApiSession) ToBoltEntityForPatch(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	return entity.ToBoltEntityForUpdate(tx, handler)
}

func (entity *ApiSession) FillFrom(handler Handler, tx *bbolt.Tx, boltEntity boltz.BaseEntity) error {
	boltApiSession, ok := boltEntity.(*persistence.ApiSession)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model ApiSession", reflect.TypeOf(boltEntity))
	}
	entity.fillCommon(boltApiSession)
	entity.Token = boltApiSession.Token
	entity.IdentityId = boltApiSession.IdentityId
	entity.ConfigTypes = stringz.SliceToSet(boltApiSession.ConfigTypes)
	boltIdentity, err := handler.GetEnv().GetStores().Identity.LoadOneById(tx, boltApiSession.IdentityId)
	if err != nil {
		return err
	}
	modelIdentity := &Identity{}
	if err := modelIdentity.FillFrom(handler, tx, boltIdentity); err != nil {
		return err
	}
	entity.Identity = modelIdentity
	return nil
}
