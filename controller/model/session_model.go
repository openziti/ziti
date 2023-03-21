/*
	Copyright NetFoundry Inc.

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
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
)

type Session struct {
	models.BaseEntity
	Token           string
	IdentityId      string
	ApiSessionId    string
	ServiceId       string
	Type            string
	ServicePolicies []string
}

func (entity *Session) toBoltEntityForCreate(tx *bbolt.Tx, manager EntityManager) (boltz.Entity, error) {
	apiSession, err := manager.GetEnv().GetStores().ApiSession.LoadOneById(tx, entity.ApiSessionId)
	if err != nil {
		return nil, err
	}
	if apiSession == nil {
		return nil, errorz.NewFieldError("api session not found", "ApiSessionId", entity.ApiSessionId)
	}

	boltEntity := &persistence.Session{
		BaseExtEntity:   *boltz.NewExtEntity(entity.Id, entity.Tags),
		Token:           entity.Token,
		ApiSessionId:    entity.ApiSessionId,
		IdentityId:      entity.IdentityId,
		ServiceId:       entity.ServiceId,
		Type:            entity.Type,
		ApiSession:      apiSession,
		ServicePolicies: entity.ServicePolicies,
	}

	return boltEntity, nil
}

func (entity *Session) toBoltEntityForUpdate(*bbolt.Tx, EntityManager, boltz.FieldChecker) (boltz.Entity, error) {
	return &persistence.Session{
		BaseExtEntity:   *boltz.NewExtEntity(entity.Id, entity.Tags),
		Token:           entity.Token,
		ApiSessionId:    entity.ApiSessionId,
		IdentityId:      entity.IdentityId,
		ServiceId:       entity.ServiceId,
		Type:            entity.Type,
		ServicePolicies: entity.ServicePolicies,
	}, nil
}

func (entity *Session) fillFrom(_ EntityManager, _ *bbolt.Tx, boltEntity boltz.Entity) error {
	boltSession, ok := boltEntity.(*persistence.Session)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model Session", reflect.TypeOf(boltEntity))
	}
	entity.FillCommon(boltSession)
	entity.Token = boltSession.Token
	entity.ApiSessionId = boltSession.ApiSessionId
	entity.IdentityId = boltSession.IdentityId
	entity.ServiceId = boltSession.ServiceId
	entity.Type = boltSession.Type
	entity.ServicePolicies = boltSession.ServicePolicies
	return nil
}
