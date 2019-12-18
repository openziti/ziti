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

type EventLog struct {
	BaseModelEntityImpl
	Type             string
	ActorType        string
	ActorId          string
	EntityType       string
	EntityId         string
	FormattedMessage string
	FormatString     string
	FormatData       string
	Data             map[string]interface{}
}

func (entity *EventLog) ToBoltEntityForCreate(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	return &persistence.EventLog{
		BaseEdgeEntityImpl: *persistence.NewBaseEdgeEntity(entity.Id, entity.Tags),
		Type:               entity.Type,
		ActorType:          entity.ActorType,
		ActorId:            entity.ActorId,
		EntityType:         entity.EntityType,
		EntityId:           entity.EntityId,
		FormattedMessage:   entity.FormattedMessage,
		FormatString:       entity.FormatString,
		FormatData:         entity.FormatData,
		Data:               entity.Data,
	}, nil
}

func (entity *EventLog) ToBoltEntityForUpdate(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	return entity.ToBoltEntityForCreate(tx, handler)
}

func (entity *EventLog) ToBoltEntityForPatch(tx *bbolt.Tx, handler Handler) (persistence.BaseEdgeEntity, error) {
	return entity.ToBoltEntityForCreate(tx, handler)
}

func (entity *EventLog) FillFrom(handler Handler, tx *bbolt.Tx, boltEntity boltz.BaseEntity) error {
	boltEventLog, ok := boltEntity.(*persistence.EventLog)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model event log", reflect.TypeOf(boltEntity))
	}
	entity.fillCommon(boltEventLog)
	entity.Type = boltEventLog.Type
	entity.ActorType = boltEventLog.ActorType
	entity.ActorId = boltEventLog.ActorId
	entity.EntityType = boltEventLog.EntityType
	entity.EntityId = boltEventLog.EntityId
	entity.FormattedMessage = boltEventLog.FormattedMessage
	entity.FormatString = boltEventLog.FormatString
	entity.FormatData = boltEventLog.FormatData
	entity.Data = boltEventLog.Data
	return nil
}
