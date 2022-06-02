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
	"github.com/openziti/foundation/util/errorz"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
	"time"
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
	Name                      string
	IdentityTypeId            string
	IsDefaultAdmin            bool
	IsAdmin                   bool
	RoleAttributes            []string
	EnvInfo                   *EnvInfo
	SdkInfo                   *SdkInfo
	HasHeartbeat              bool
	DefaultHostingPrecedence  ziti.Precedence
	DefaultHostingCost        uint16
	ServiceHostingPrecedences map[string]ziti.Precedence
	ServiceHostingCosts       map[string]uint16
	AppData                   map[string]interface{}
	AuthPolicyId              string
	ExternalId                *string
	Disabled                  bool
	DisabledAt                *time.Time
	DisabledUntil             *time.Time
}

func (entity *Identity) toBoltEntityForCreate(_ *bbolt.Tx, _ EntityManager) (boltz.Entity, error) {
	boltEntity := &persistence.Identity{
		BaseExtEntity:             *boltz.NewExtEntity(entity.Id, entity.Tags),
		Name:                      entity.Name,
		IdentityTypeId:            entity.IdentityTypeId,
		AuthPolicyId:              entity.AuthPolicyId,
		IsDefaultAdmin:            entity.IsDefaultAdmin,
		IsAdmin:                   entity.IsAdmin,
		RoleAttributes:            entity.RoleAttributes,
		DefaultHostingPrecedence:  entity.DefaultHostingPrecedence,
		DefaultHostingCost:        entity.DefaultHostingCost,
		ServiceHostingPrecedences: entity.ServiceHostingPrecedences,
		ServiceHostingCosts:       entity.ServiceHostingCosts,
		AppData:                   entity.AppData,
		ExternalId:                entity.ExternalId,
		DisabledAt:                entity.DisabledAt,
		DisabledUntil:             entity.DisabledUntil,
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

func (entity *Identity) toBoltEntityForUpdate(tx *bbolt.Tx, handler EntityManager) (boltz.Entity, error) {
	return entity.toBoltEntityForChange(tx, handler, nil)
}

func (entity *Identity) toBoltEntityForPatch(tx *bbolt.Tx, handler EntityManager, checker boltz.FieldChecker) (boltz.Entity, error) {
	return entity.toBoltEntityForChange(tx, handler, checker)
}

func (entity *Identity) toBoltEntityForChange(tx *bbolt.Tx, handler EntityManager, checker boltz.FieldChecker) (boltz.Entity, error) {
	boltEntity := &persistence.Identity{
		Name:                      entity.Name,
		IdentityTypeId:            entity.IdentityTypeId,
		AuthPolicyId:              entity.AuthPolicyId,
		BaseExtEntity:             *boltz.NewExtEntity(entity.Id, entity.Tags),
		RoleAttributes:            entity.RoleAttributes,
		DefaultHostingPrecedence:  entity.DefaultHostingPrecedence,
		DefaultHostingCost:        entity.DefaultHostingCost,
		ServiceHostingPrecedences: entity.ServiceHostingPrecedences,
		ServiceHostingCosts:       entity.ServiceHostingCosts,
		AppData:                   entity.AppData,
		ExternalId:                entity.ExternalId,
		DisabledAt:                entity.DisabledAt,
		DisabledUntil:             entity.DisabledUntil,
		IsAdmin:                   entity.IsAdmin,
	}

	_, currentType := handler.GetStore().GetSymbol(persistence.FieldIdentityType).Eval(tx, []byte(entity.Id))
	if string(currentType) == persistence.RouterIdentityType {
		if (checker == nil || checker.IsUpdated("identityTypeId")) && entity.IdentityTypeId != persistence.RouterIdentityType {
			fieldErr := errorz.NewFieldError("may not change type of router identities", "typeId", entity.IdentityTypeId)
			return nil, errorz.NewFieldApiError(fieldErr)
		}

		_, currentName := handler.GetStore().GetSymbol(persistence.FieldName).Eval(tx, []byte(entity.Id))
		if (checker == nil || checker.IsUpdated(persistence.FieldName)) && string(currentName) != entity.Name {
			fieldErr := errorz.NewFieldError("may not change name of router identities", "name", entity.Name)
			return nil, errorz.NewFieldApiError(fieldErr)
		}
	} else if (checker == nil || checker.IsUpdated("identityTypeId")) && entity.IdentityTypeId == persistence.RouterIdentityType {
		fieldErr := errorz.NewFieldError("may not change type to router", "typeId", entity.IdentityTypeId)
		return nil, errorz.NewFieldApiError(fieldErr)
	}

	fillPersistenceInfo(boltEntity, entity.EnvInfo, entity.SdkInfo)

	return boltEntity, nil
}

func (entity *Identity) fillFrom(handler EntityManager, tx *bbolt.Tx, boltEntity boltz.Entity) error {
	boltIdentity, ok := boltEntity.(*persistence.Identity)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model identity", reflect.TypeOf(boltEntity))
	}
	entity.FillCommon(boltIdentity)
	entity.Name = boltIdentity.Name
	entity.IdentityTypeId = boltIdentity.IdentityTypeId
	entity.AuthPolicyId = boltIdentity.AuthPolicyId
	entity.IsDefaultAdmin = boltIdentity.IsDefaultAdmin
	entity.IsAdmin = boltIdentity.IsAdmin
	entity.RoleAttributes = boltIdentity.RoleAttributes
	entity.HasHeartbeat = handler.GetEnv().GetManagers().Identity.IsActive(entity.Id)
	entity.DefaultHostingPrecedence = boltIdentity.DefaultHostingPrecedence
	entity.DefaultHostingCost = boltIdentity.DefaultHostingCost
	entity.ServiceHostingPrecedences = boltIdentity.ServiceHostingPrecedences
	entity.ServiceHostingCosts = boltIdentity.ServiceHostingCosts
	entity.AppData = boltIdentity.AppData
	entity.ExternalId = boltIdentity.ExternalId
	entity.DisabledUntil = boltIdentity.DisabledUntil
	entity.DisabledAt = boltIdentity.DisabledAt
	entity.Disabled = boltIdentity.Disabled
	fillModelInfo(entity, boltIdentity.EnvInfo, boltIdentity.SdkInfo)

	return nil
}

type ServiceConfig struct {
	Service string
	Config  string
}

func toBoltServiceConfigs(tx *bbolt.Tx, handler EntityManager, serviceConfigs []ServiceConfig) ([]persistence.ServiceConfig, error) {
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
