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
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/models"
	"go.etcd.io/bbolt"
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

func (entity *Session) toBoltEntityForCreate(tx *bbolt.Tx, env Env) (*db.Session, error) {
	apiSession, err := env.GetStores().ApiSession.LoadOneById(tx, entity.ApiSessionId)
	if err != nil {
		return nil, err
	}
	if apiSession == nil {
		return nil, errorz.NewFieldError("api session not found", "ApiSessionId", entity.ApiSessionId)
	}

	boltEntity := &db.Session{
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

func (entity *Session) toBoltEntityForUpdate(*bbolt.Tx, Env, boltz.FieldChecker) (*db.Session, error) {
	return &db.Session{
		BaseExtEntity:   *boltz.NewExtEntity(entity.Id, entity.Tags),
		Token:           entity.Token,
		ApiSessionId:    entity.ApiSessionId,
		IdentityId:      entity.IdentityId,
		ServiceId:       entity.ServiceId,
		Type:            entity.Type,
		ServicePolicies: entity.ServicePolicies,
	}, nil
}

func (entity *Session) fillFrom(_ Env, _ *bbolt.Tx, boltSession *db.Session) error {
	entity.FillCommon(boltSession)
	entity.Token = boltSession.Token
	entity.ApiSessionId = boltSession.ApiSessionId
	entity.IdentityId = boltSession.IdentityId
	entity.ServiceId = boltSession.ServiceId
	entity.Type = boltSession.Type
	entity.ServicePolicies = boltSession.ServicePolicies
	return nil
}
