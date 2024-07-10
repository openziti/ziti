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
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/models"
	"go.etcd.io/bbolt"
	"time"
)

type Service struct {
	models.BaseEntity
	Name               string
	TerminatorStrategy string
	Terminators        []*Terminator
	MaxIdleTime        time.Duration
}

func (entity *Service) GetName() string {
	return entity.Name
}

func (entity *Service) toBoltEntityForUpdate(tx *bbolt.Tx, env Env, _ boltz.FieldChecker) (*db.Service, error) {
	return entity.toBoltEntityForCreate(tx, env)
}

func (entity *Service) toBoltEntityForCreate(*bbolt.Tx, Env) (*db.Service, error) {
	return &db.Service{
		BaseExtEntity:      *boltz.NewExtEntity(entity.Id, entity.Tags),
		Name:               entity.Name,
		MaxIdleTime:        entity.MaxIdleTime,
		TerminatorStrategy: entity.TerminatorStrategy,
	}, nil
}

func (entity *Service) fillFrom(env Env, tx *bbolt.Tx, boltService *db.Service) error {
	entity.Name = boltService.Name
	entity.MaxIdleTime = boltService.MaxIdleTime
	entity.TerminatorStrategy = boltService.TerminatorStrategy
	entity.FillCommon(boltService)

	terminatorIds := env.GetStores().Service.GetRelatedEntitiesIdList(tx, entity.Id, db.EntityTypeTerminators)
	for _, terminatorId := range terminatorIds {
		if terminator, _ := env.GetManagers().Terminator.readInTx(tx, terminatorId); terminator != nil {
			entity.Terminators = append(entity.Terminators, terminator)
		}
	}

	return nil
}
