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
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/models"
	"go.etcd.io/bbolt"
	"time"
)

type EnvInfo struct {
	Arch      string
	Os        string
	OsRelease string
	OsVersion string
}

func (self *EnvInfo) Equals(other *EnvInfo) bool {
	if self == nil {
		return other == nil
	}
	if other == nil {
		return false
	}
	return self.Arch == other.Arch &&
		self.Os == other.Os &&
		self.OsRelease == other.OsRelease &&
		self.OsVersion == other.OsVersion
}

type SdkInfo struct {
	AppId      string
	AppVersion string
	Branch     string
	Revision   string
	Type       string
	Version    string
}

func (self *SdkInfo) Equals(other *SdkInfo) bool {
	if self == nil {
		return other == nil
	}
	if other == nil {
		return false
	}
	return self.AppId == other.AppId &&
		self.AppVersion == other.AppVersion &&
		self.Branch == other.Branch &&
		self.Revision == other.Revision &&
		self.Type == other.Type &&
		self.Version == other.Version
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
	HasErConnection           bool
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

func (entity *Identity) toBoltEntityForCreate(_ *bbolt.Tx, env Env) (*db.Identity, error) {
	identityType, err := env.GetManagers().IdentityType.ReadByIdOrName(entity.IdentityTypeId)

	if err != nil && !boltz.IsErrNotFoundErr(err) {
		return nil, err
	}

	if identityType == nil {
		apiErr := errorz.NewNotFound()
		apiErr.Cause = errorz.NewFieldError("typeId not found", "typeId", entity.IdentityTypeId)
		apiErr.AppendCause = true
		return nil, apiErr
	}

	if identityType.Name == db.RouterIdentityType {
		fieldErr := errorz.NewFieldError("may not create identities with given typeId", "typeId", entity.IdentityTypeId)
		return nil, errorz.NewFieldApiError(fieldErr)
	}

	entity.IdentityTypeId = identityType.Id

	boltEntity := &db.Identity{
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
		boltEntity.EnvInfo = &db.EnvInfo{
			Arch:      entity.EnvInfo.Arch,
			Os:        entity.EnvInfo.Os,
			OsRelease: entity.EnvInfo.OsRelease,
			OsVersion: entity.EnvInfo.OsVersion,
		}
	}

	if entity.SdkInfo != nil {
		boltEntity.SdkInfo = &db.SdkInfo{
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

func fillModelInfo(identity *Identity, envInfo *db.EnvInfo, sdkInfo *db.SdkInfo) {
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

func fillPersistenceInfo(identity *db.Identity, envInfo *EnvInfo, sdkInfo *SdkInfo) {
	if envInfo != nil {
		identity.EnvInfo = &db.EnvInfo{
			Arch:      envInfo.Arch,
			Os:        envInfo.Os,
			OsRelease: envInfo.OsRelease,
			OsVersion: envInfo.OsVersion,
		}
	}

	if sdkInfo != nil {
		identity.SdkInfo = &db.SdkInfo{
			Branch:     sdkInfo.Branch,
			Revision:   sdkInfo.Revision,
			Type:       sdkInfo.Type,
			Version:    sdkInfo.Version,
			AppId:      sdkInfo.AppId,
			AppVersion: sdkInfo.AppVersion,
		}
	}
}

func (entity *Identity) toBoltEntityForUpdate(tx *bbolt.Tx, env Env, checker boltz.FieldChecker) (*db.Identity, error) {
	if checker == nil || checker.IsUpdated("type") {
		identityType, err := env.GetManagers().IdentityType.ReadByIdOrName(entity.IdentityTypeId)

		if err != nil && !boltz.IsErrNotFoundErr(err) {
			return nil, err
		}

		if identityType == nil {
			apiErr := errorz.NewNotFound()
			apiErr.Cause = errorz.NewFieldError("identityTypeId not found", "identityTypeId", entity.IdentityTypeId)
			apiErr.AppendCause = true
			return nil, apiErr
		}

		entity.IdentityTypeId = identityType.Id
	}

	boltEntity := &db.Identity{
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

	identityStore := env.GetManagers().Identity.GetStore()
	_, currentType := identityStore.GetSymbol(db.FieldIdentityType).Eval(tx, []byte(entity.Id))
	if string(currentType) == db.RouterIdentityType {
		if (checker == nil || checker.IsUpdated("identityTypeId")) && entity.IdentityTypeId != db.RouterIdentityType {
			fieldErr := errorz.NewFieldError("may not change type of router identities", "typeId", entity.IdentityTypeId)
			return nil, errorz.NewFieldApiError(fieldErr)
		}

		_, currentName := identityStore.GetSymbol(db.FieldName).Eval(tx, []byte(entity.Id))
		if (checker == nil || checker.IsUpdated(db.FieldName)) && string(currentName) != entity.Name {
			fieldErr := errorz.NewFieldError("may not change name of router identities", "name", entity.Name)
			return nil, errorz.NewFieldApiError(fieldErr)
		}
	} else if (checker == nil || checker.IsUpdated("identityTypeId")) && entity.IdentityTypeId == db.RouterIdentityType {
		fieldErr := errorz.NewFieldError("may not change type to router", "typeId", entity.IdentityTypeId)
		return nil, errorz.NewFieldApiError(fieldErr)
	}

	fillPersistenceInfo(boltEntity, entity.EnvInfo, entity.SdkInfo)

	return boltEntity, nil
}

func (entity *Identity) fillFrom(env Env, _ *bbolt.Tx, boltIdentity *db.Identity) error {
	entity.FillCommon(boltIdentity)
	entity.Name = boltIdentity.Name
	entity.IdentityTypeId = boltIdentity.IdentityTypeId
	entity.AuthPolicyId = boltIdentity.AuthPolicyId
	entity.IsDefaultAdmin = boltIdentity.IsDefaultAdmin
	entity.IsAdmin = boltIdentity.IsAdmin
	entity.RoleAttributes = boltIdentity.RoleAttributes
	entity.HasErConnection = env.GetManagers().Identity.HasErConnection(entity.Id)
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

func toBoltServiceConfigs(tx *bbolt.Tx, env Env, serviceConfigs []ServiceConfig) ([]db.ServiceConfig, error) {
	serviceStore := env.GetStores().EdgeService
	configStore := env.GetStores().Config

	var boltServiceConfigs []db.ServiceConfig
	for _, serviceConfig := range serviceConfigs {
		if !serviceStore.IsEntityPresent(tx, serviceConfig.Service) {
			return nil, boltz.NewNotFoundError(serviceStore.GetSingularEntityType(), "id or name", serviceConfig.Service)
		}

		if !configStore.IsEntityPresent(tx, serviceConfig.Config) {
			return nil, boltz.NewNotFoundError(configStore.GetSingularEntityType(), "id or name", serviceConfig.Config)
		}

		boltServiceConfigs = append(boltServiceConfigs, db.ServiceConfig{
			ServiceId: serviceConfig.Service,
			ConfigId:  serviceConfig.Config,
		})
	}
	return boltServiceConfigs, nil
}
