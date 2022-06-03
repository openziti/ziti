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
	"fmt"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/util/errorz"
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
)

type Service struct {
	models.BaseEntity
	Name               string   `json:"name"`
	TerminatorStrategy string   `json:"terminatorStrategy"`
	RoleAttributes     []string `json:"roleAttributes"`
	Configs            []string `json:"configs"`
	EncryptionRequired bool     `json:"encryptionRequired"`
}

func (entity *Service) toBoltEntity(tx *bbolt.Tx, handler EntityManager) (boltz.Entity, error) {
	if err := entity.validateConfigs(tx, handler); err != nil {
		return nil, err
	}

	edgeService := &persistence.EdgeService{
		Service: db.Service{
			BaseExtEntity:      *boltz.NewExtEntity(entity.Id, entity.Tags),
			Name:               entity.Name,
			TerminatorStrategy: entity.TerminatorStrategy,
		},
		RoleAttributes:     entity.RoleAttributes,
		Configs:            entity.Configs,
		EncryptionRequired: entity.EncryptionRequired,
	}
	return edgeService, nil
}

func (entity *Service) toBoltEntityForCreate(tx *bbolt.Tx, handler EntityManager) (boltz.Entity, error) {
	return entity.toBoltEntity(tx, handler)
}

func (entity *Service) validateConfigs(tx *bbolt.Tx, handler EntityManager) error {
	typeMap := map[string]*persistence.Config{}
	configStore := handler.GetEnv().GetStores().Config
	for _, id := range entity.Configs {
		config, _ := configStore.LoadOneById(tx, id)
		if config == nil {
			return boltz.NewNotFoundError(persistence.EntityTypeConfigs, "id", id)
		}
		conflictConfig, found := typeMap[config.Type]
		if found {
			configTypeName := "<not found>"
			if configType, _ := handler.GetEnv().GetStores().ConfigType.LoadOneById(tx, config.Type); configType != nil {
				configTypeName = configType.Name
			}
			msg := fmt.Sprintf("duplicate configs named %v and %v found for config type %v. Only one config of a given typed is allowed per service ",
				conflictConfig.Name, config.Name, configTypeName)
			return errorz.NewFieldError(msg, "configs", entity.Configs)
		}
		typeMap[config.Type] = config
	}
	return nil
}

func (entity *Service) toBoltEntityForUpdate(tx *bbolt.Tx, handler EntityManager) (boltz.Entity, error) {
	return entity.toBoltEntity(tx, handler)
}

func (entity *Service) toBoltEntityForPatch(tx *bbolt.Tx, handler EntityManager, checker boltz.FieldChecker) (boltz.Entity, error) {
	return entity.toBoltEntity(tx, handler)
}

func (entity *Service) fillFrom(_ EntityManager, _ *bbolt.Tx, boltEntity boltz.Entity) error {
	boltService, ok := boltEntity.(*persistence.EdgeService)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model service", reflect.TypeOf(boltEntity))
	}
	entity.FillCommon(boltService)
	entity.Name = boltService.Name
	entity.TerminatorStrategy = boltService.TerminatorStrategy
	entity.RoleAttributes = boltService.RoleAttributes
	entity.Configs = boltService.Configs
	entity.EncryptionRequired = boltService.EncryptionRequired
	return nil
}

type ServiceDetail struct {
	models.BaseEntity
	Name               string                            `json:"name"`
	TerminatorStrategy string                            `json:"terminatorStrategy"`
	RoleAttributes     []string                          `json:"roleAttributes"`
	Permissions        []string                          `json:"permissions"`
	Configs            []string                          `json:"configs"`
	Config             map[string]map[string]interface{} `json:"config"`
	EncryptionRequired bool                              `json:"encryptionRequired"`
}

func (entity *ServiceDetail) fillFrom(_ EntityManager, _ *bbolt.Tx, boltEntity boltz.Entity) error {
	boltService, ok := boltEntity.(*persistence.EdgeService)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model service", reflect.TypeOf(boltEntity))
	}
	entity.FillCommon(boltService)
	entity.Name = boltService.Name
	entity.TerminatorStrategy = boltService.TerminatorStrategy
	entity.RoleAttributes = boltService.RoleAttributes
	entity.Configs = boltService.Configs
	entity.EncryptionRequired = boltService.EncryptionRequired

	return nil
}
