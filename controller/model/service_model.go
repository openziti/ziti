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
	"time"

	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/openziti/ziti/v2/controller/db"
	"github.com/openziti/ziti/v2/controller/models"
	"go.etcd.io/bbolt"
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
	// Since the edge/fabric service collapse, the edge fields (RoleAttributes, Configs,
	// EncryptionRequired) and the IsFabricOnly discriminator share the service entity bucket with
	// the fabric fields. Load the existing entity and overlay only the fabric-owned fields so a
	// fabric update preserves the rest. Without this, a full PUT (nil field checker) would persist
	// zero values for the edge fields and flip an existing edge service to fabric-only.
	existing, err := env.GetStores().Service.LoadById(tx, entity.Id)
	if err != nil {
		return nil, err
	}
	existing.Name = entity.Name
	existing.MaxIdleTime = entity.MaxIdleTime
	existing.TerminatorStrategy = entity.TerminatorStrategy
	existing.Tags = entity.Tags
	return existing, nil
}

func (entity *Service) toBoltEntityForCreate(*bbolt.Tx, Env) (*db.Service, error) {
	return &db.Service{
		BaseExtEntity:      *boltz.NewExtEntity(entity.Id, entity.Tags),
		Name:               entity.Name,
		MaxIdleTime:        entity.MaxIdleTime,
		TerminatorStrategy: entity.TerminatorStrategy,
		IsFabricOnly:       true,
		// The fabric surface does not model encryptionRequired, so apply the secure default.
		// The field is unreachable for fabric-only services today, but when fabric and edge
		// services fully merge, fabric services should require encryption unless explicitly
		// opted out.
		EncryptionRequired: true,
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
