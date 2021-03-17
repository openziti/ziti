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
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
)

type EnvInfo struct {
	Arch      string
	Os        string
	OsRelease string
	OsVersion string
}

type SdkInfo struct {
	AppId      string
	AppVersion string
	Branch     string
	Revision   string
	Type       string
	Version    string
}

type Identity struct {
	models.BaseEntity
	Name                     string
	IdentityTypeId           string
	IsDefaultAdmin           bool
	IsAdmin                  bool
	RoleAttributes           []string
	EnvInfo                  *EnvInfo
	SdkInfo                  *SdkInfo
	HasHeartbeat             bool
	DefaultHostingPrecedence ziti.Precedence
	DefaultHostingCost       uint16
}

func (entity *Identity) toBoltEntityForCreate(_ *bbolt.Tx, _ Handler) (boltz.Entity, error) {
	boltEntity := &persistence.Identity{
		BaseExtEntity:            *boltz.NewExtEntity(entity.Id, entity.Tags),
		Name:                     entity.Name,
		IdentityTypeId:           entity.IdentityTypeId,
		IsDefaultAdmin:           entity.IsDefaultAdmin,
		IsAdmin:                  entity.IsAdmin,
		RoleAttributes:           entity.RoleAttributes,
		DefaultHostingPrecedence: entity.DefaultHostingPrecedence,
		DefaultHostingCost:       entity.DefaultHostingCost,
	}

	if entity.EnvInfo != nil {
		boltEntity.EnvInfo = &persistence.EnvInfo{
			Arch:      entity.EnvInfo.Arch,
			Os:        entity.EnvInfo.Os,
			OsRelease: entity.EnvInfo.OsRelease,
			OsVersion: entity.EnvInfo.OsVersion,
		}
	}

	if entity.SdkInfo != nil {
		boltEntity.SdkInfo = &persistence.SdkInfo{
			Branch:     entity.SdkInfo.Branch,
			Revision:   entity.SdkInfo.Revision,
			Type:       entity.SdkInfo.Type,
			Version:    entity.SdkInfo.Version,
			AppId:      entity.SdkInfo.AppId,
			AppVersion: entity.SdkInfo.AppVersion,
		}
	}
	fillPersistenceInfo(boltEntity, entity.EnvInfo, entity.SdkInfo)

	return boltEntity, nil
}

func fillModelInfo(identity *Identity, envInfo *persistence.EnvInfo, sdkInfo *persistence.SdkInfo) {
	if envInfo != nil {
		identity.EnvInfo = &EnvInfo{
			Arch:      envInfo.Arch,
			Os:        envInfo.Os,
			OsRelease: envInfo.OsRelease,
			OsVersion: envInfo.OsVersion,
		}
	}

	if sdkInfo != nil {
		identity.SdkInfo = &SdkInfo{
			AppId:      sdkInfo.AppId,
			AppVersion: sdkInfo.AppVersion,
			Branch:     sdkInfo.Branch,
			Revision:   sdkInfo.Revision,
			Type:       sdkInfo.Type,
			Version:    sdkInfo.Version,
		}
	}
}

func fillPersistenceInfo(identity *persistence.Identity, envInfo *EnvInfo, sdkInfo *SdkInfo) {
	if envInfo != nil {
		identity.EnvInfo = &persistence.EnvInfo{
			Arch:      envInfo.Arch,
			Os:        envInfo.Os,
			OsRelease: envInfo.OsRelease,
			OsVersion: envInfo.OsVersion,
		}
	}

	if sdkInfo != nil {
		identity.SdkInfo = &persistence.SdkInfo{
			Branch:     sdkInfo.Branch,
			Revision:   sdkInfo.Revision,
			Type:       sdkInfo.Type,
			Version:    sdkInfo.Version,
			AppId:      sdkInfo.AppId,
			AppVersion: sdkInfo.AppVersion,
		}
	}
}

func (entity *Identity) toBoltEntityForUpdate(*bbolt.Tx, Handler) (boltz.Entity, error) {
	boltEntity := &persistence.Identity{
		Name:                     entity.Name,
		IdentityTypeId:           entity.IdentityTypeId,
		BaseExtEntity:            *boltz.NewExtEntity(entity.Id, entity.Tags),
		RoleAttributes:           entity.RoleAttributes,
		DefaultHostingPrecedence: entity.DefaultHostingPrecedence,
		DefaultHostingCost:       entity.DefaultHostingCost,
	}

	fillPersistenceInfo(boltEntity, entity.EnvInfo, entity.SdkInfo)

	return boltEntity, nil
}

func (entity *Identity) toBoltEntityForPatch(tx *bbolt.Tx, handler Handler, _ boltz.FieldChecker) (boltz.Entity, error) {
	return entity.toBoltEntityForUpdate(tx, handler)
}

func (entity *Identity) fillFrom(handler Handler, tx *bbolt.Tx, boltEntity boltz.Entity) error {
	boltIdentity, ok := boltEntity.(*persistence.Identity)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model identity", reflect.TypeOf(boltEntity))
	}
	entity.FillCommon(boltIdentity)
	entity.Name = boltIdentity.Name
	entity.IdentityTypeId = boltIdentity.IdentityTypeId
	entity.IsDefaultAdmin = boltIdentity.IsDefaultAdmin
	entity.IsAdmin = boltIdentity.IsAdmin
	entity.RoleAttributes = boltIdentity.RoleAttributes
	entity.HasHeartbeat = handler.GetEnv().GetHandlers().Identity.IsActive(entity.Id)
	entity.DefaultHostingPrecedence = boltIdentity.DefaultHostingPrecedence
	entity.DefaultHostingCost = boltIdentity.DefaultHostingCost

	fillModelInfo(entity, boltIdentity.EnvInfo, boltIdentity.SdkInfo)

	return nil
}

type ServiceConfig struct {
	Service string
	Config  string
}

func toBoltServiceConfigs(tx *bbolt.Tx, handler Handler, serviceConfigs []ServiceConfig) ([]persistence.ServiceConfig, error) {
	serviceStore := handler.GetEnv().GetStores().EdgeService
	configStore := handler.GetEnv().GetStores().Config

	var boltServiceConfigs []persistence.ServiceConfig
	for _, serviceConfig := range serviceConfigs {
		if !serviceStore.IsEntityPresent(tx, serviceConfig.Service) {
			return nil, boltz.NewNotFoundError(serviceStore.GetSingularEntityType(), "id or name", serviceConfig.Service)
		}

		if !configStore.IsEntityPresent(tx, serviceConfig.Config) {
			return nil, boltz.NewNotFoundError(configStore.GetSingularEntityType(), "id or name", serviceConfig.Config)
		}

		boltServiceConfigs = append(boltServiceConfigs, persistence.ServiceConfig{
			ServiceId: serviceConfig.Service,
			ConfigId:  serviceConfig.Config,
		})
	}
	return boltServiceConfigs, nil
}
