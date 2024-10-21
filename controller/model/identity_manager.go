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
	"encoding/json"
	"errors"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v3"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/metrics"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/common/inspect"
	"github.com/openziti/ziti/common/pb/cmd_pb"
	"github.com/openziti/ziti/common/pb/edge_cmd_pb"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/command"
	"github.com/openziti/ziti/controller/config"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/event"
	"github.com/openziti/ziti/controller/fields"
	"github.com/openziti/ziti/controller/models"
	cmap "github.com/orcaman/concurrent-map/v2"
	"go.etcd.io/bbolt"
	"google.golang.org/protobuf/proto"
	"sync"
	"time"
)

const (
	IdentityActiveIntervalSeconds = 60

	minDefaultAdminPasswordLength = 5
	maxDefaultAdminPasswordLength = 100
	minDefaultAdminUsernameLength = 4
	maxDefaultAdminUsernameLength = 100
	minDefaultAdminNameLength     = 4
	maxDefaultAdminNameLength     = 100
)

type IdentityManager struct {
	baseEntityManager[*Identity, *db.Identity]
	updateSdkInfoTimer metrics.Timer
	identityStatusMap  *identityStatusMap
	connections        *ConnectionTracker
	statusSource       config.IdentityStatusSource
}

func NewIdentityManager(env Env) *IdentityManager {
	manager := &IdentityManager{
		baseEntityManager:  newBaseEntityManager[*Identity, *db.Identity](env, env.GetStores().Identity),
		updateSdkInfoTimer: env.GetMetricsRegistry().Timer("identity.update-sdk-info"),
		identityStatusMap:  newIdentityStatusMap(IdentityActiveIntervalSeconds * time.Second),
		connections:        newConnectionTracker(env),
		statusSource:       env.GetConfig().Edge.IdentityStatusConfig.Source,
	}
	manager.impl = manager

	RegisterManagerDecoder[*Identity](env, manager)
	RegisterCommand(env, &CreateIdentityWithEnrollmentsCmd{}, &edge_cmd_pb.CreateIdentityWithEnrollmentsCmd{})
	RegisterCommand(env, &UpdateServiceConfigsCmd{}, &edge_cmd_pb.UpdateServiceConfigsCmd{})

	return manager
}

func (self *IdentityManager) newModelEntity() *Identity {
	return &Identity{}
}

func (self *IdentityManager) Create(entity *Identity, ctx *change.Context) error {
	return DispatchCreate[*Identity](self, entity, ctx)
}

func (self *IdentityManager) ApplyCreate(cmd *command.CreateEntityCommand[*Identity], ctx boltz.MutateContext) error {
	_, err := self.createEntity(cmd.Entity, ctx)
	return err
}

func (self *IdentityManager) CreateWithEnrollments(identityModel *Identity, enrollmentsModels []*Enrollment, ctx *change.Context) error {
	if identityModel.Id == "" {
		identityModel.Id = eid.New()
	}

	for _, enrollment := range enrollmentsModels {
		if enrollment.Id == "" {
			enrollment.Id = eid.New()
		}
		enrollment.IdentityId = &identityModel.Id
	}

	cmd := &CreateIdentityWithEnrollmentsCmd{
		manager:     self,
		identity:    identityModel,
		enrollments: enrollmentsModels,
		ctx:         ctx,
	}

	return self.Dispatch(cmd)
}

func (self *IdentityManager) ApplyCreateWithEnrollments(cmd *CreateIdentityWithEnrollmentsCmd, ctx boltz.MutateContext) error {
	identityModel := cmd.identity
	enrollmentsModels := cmd.enrollments

	return self.GetDb().Update(ctx, func(ctx boltz.MutateContext) error {
		boltEntity, err := identityModel.toBoltEntityForCreate(ctx.Tx(), self.env)
		if err != nil {
			return err
		}
		if err = self.GetStore().Create(ctx, boltEntity); err != nil {
			pfxlog.Logger().WithError(err).Errorf("could not create %v in bolt storage", self.GetStore().GetSingularEntityType())
			return err
		}

		for _, enrollment := range enrollmentsModels {
			enrollment.IdentityId = &identityModel.Id

			if err = enrollment.FillJwtInfo(self.env, identityModel.Id); err != nil {
				return err
			}

			if _, err = self.env.GetManagers().Enrollment.createEntityInTx(ctx, enrollment); err != nil {
				return err
			}
		}
		return nil
	})
}

func (self *IdentityManager) Update(entity *Identity, checker fields.UpdatedFields, ctx *change.Context) error {
	return DispatchUpdate[*Identity](self, entity, checker, ctx)
}

func (self *IdentityManager) ApplyUpdate(cmd *command.UpdateEntityCommand[*Identity], ctx boltz.MutateContext) error {
	var checker boltz.FieldChecker

	if cmd.UpdatedFields == nil {
		checker = &AndFieldChecker{
			first: self,
			second: NotFieldChecker{
				db.FieldIdentityServiceConfigs: struct{}{},
			},
		}
	} else {
		checker = &AndFieldChecker{first: self, second: cmd.UpdatedFields}
	}
	return self.updateEntity(cmd.Entity, checker, ctx)
}

func (self *IdentityManager) IsUpdated(field string) bool {
	return field != db.FieldIdentityAuthenticators && field != db.FieldIdentityEnrollments && field != db.FieldIdentityIsDefaultAdmin
}

func (self *IdentityManager) ReadByName(name string) (*Identity, error) {
	entity := &Identity{}
	nameIndex := self.env.GetStores().Identity.GetNameIndex()
	if err := self.readEntityWithIndex("name", []byte(name), nameIndex, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (self *IdentityManager) ReadDefaultAdmin() (*Identity, error) {
	return self.ReadOneByQuery("isDefaultAdmin = true")
}

func (self *IdentityManager) ReadOneByQuery(query string) (*Identity, error) {
	result, err := self.readEntityByQuery(query)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return result.(*Identity), nil
}

func (self *IdentityManager) InitializeDefaultAdmin(username, password, name string) error {
	if len(username) < minDefaultAdminUsernameLength {
		return errorz.NewFieldError(fmt.Sprintf("username must be at least %v characters", minDefaultAdminUsernameLength), "username", username)
	}
	if len(password) < minDefaultAdminPasswordLength {
		return errorz.NewFieldError(fmt.Sprintf("password must be at least %v characters", minDefaultAdminPasswordLength), "password", "******")
	}
	if len(name) < minDefaultAdminNameLength {
		return errorz.NewFieldError(fmt.Sprintf("name must be at least %v characters", minDefaultAdminNameLength), "name", name)
	}

	if len(username) > maxDefaultAdminUsernameLength {
		return errorz.NewFieldError(fmt.Sprintf("username must be at most %v characters", maxDefaultAdminUsernameLength), "username", username)
	}
	if len(password) > maxDefaultAdminPasswordLength {
		return errorz.NewFieldError(fmt.Sprintf("password must be at most %v characters", maxDefaultAdminPasswordLength), "password", "******")
	}
	if len(name) > maxDefaultAdminNameLength {
		return errorz.NewFieldError(fmt.Sprintf("name must be at most %v characters", maxDefaultAdminNameLength), "name", name)
	}

	identityType, err := self.env.GetManagers().IdentityType.ReadByName(db.DefaultIdentityType)

	if err != nil {
		return err
	}

	identity, err := self.ReadDefaultAdmin()

	if err != nil && !boltz.IsErrNotFoundErr(err) {
		return err
	}

	if identity != nil {
		return errors.New("already initialized: Ziti Edge default admin already defined")
	}

	if err = self.env.GetManagers().Dispatcher.Bootstrap(); err != nil {
		return fmt.Errorf("unable to bootstrap command dispatcher (%w)", err)
	}

	identityId := eid.New()
	authenticatorId := eid.New()

	defaultAdmin := &Identity{
		BaseEntity: models.BaseEntity{
			Id: identityId,
		},
		Name:           name,
		IdentityTypeId: identityType.Id,
		IsDefaultAdmin: true,
		IsAdmin:        true,
	}

	authenticator := &Authenticator{
		BaseEntity: models.BaseEntity{
			Id: authenticatorId,
		},
		Method:     db.MethodAuthenticatorUpdb,
		IdentityId: identityId,
		SubType: &AuthenticatorUpdb{
			Username: username,
			Password: password,
		},
	}

	ctx := change.New().SetSourceType("cli.init").SetChangeAuthorType(change.AuthorTypeController)
	if err = self.Create(defaultAdmin, ctx); err != nil {
		return err
	}

	if err = self.env.GetManagers().Authenticator.Create(authenticator, ctx); err != nil {
		return err
	}

	return nil
}

func (self *IdentityManager) CollectAuthenticators(id string, collector func(entity *Authenticator) error) error {
	return self.GetDb().View(func(tx *bbolt.Tx) error {
		_, err := self.readInTx(tx, id)
		if err != nil {
			return err
		}
		authenticatorIds := self.GetStore().GetRelatedEntitiesIdList(tx, id, db.FieldIdentityAuthenticators)
		for _, authenticatorId := range authenticatorIds {
			authenticator := &Authenticator{}
			err := self.env.GetManagers().Authenticator.readEntityInTx(tx, authenticatorId, authenticator)
			if err != nil {
				return err
			}
			if err = collector(authenticator); err != nil {
				return err
			}
		}
		return nil
	})
}

func (self *IdentityManager) visitAuthenticators(tx *bbolt.Tx, id string, visitor func(entity *Authenticator) bool) error {
	_, err := self.readInTx(tx, id)
	if err != nil {
		return err
	}
	authenticatorIds := self.GetStore().GetRelatedEntitiesIdList(tx, id, db.FieldIdentityAuthenticators)
	for _, authenticatorId := range authenticatorIds {
		authenticator := &Authenticator{}
		if err := self.env.GetManagers().Authenticator.readEntityInTx(tx, authenticatorId, authenticator); err != nil {
			return err
		}
		if visitor(authenticator) {
			return nil
		}
	}
	return nil

}

func (self *IdentityManager) CollectEnrollments(id string, collector func(entity *Enrollment) error) error {
	return self.GetDb().View(func(tx *bbolt.Tx) error {
		return self.collectEnrollmentsInTx(tx, id, collector)
	})
}

func (self *IdentityManager) collectEnrollmentsInTx(tx *bbolt.Tx, id string, collector func(entity *Enrollment) error) error {
	_, err := self.readInTx(tx, id)
	if err != nil {
		return err
	}

	associationIds := self.GetStore().GetRelatedEntitiesIdList(tx, id, db.FieldIdentityEnrollments)
	for _, enrollmentId := range associationIds {
		enrollment, err := self.env.GetManagers().Enrollment.readInTx(tx, enrollmentId)
		if err != nil {
			return err
		}
		err = collector(enrollment)

		if err != nil {
			return err
		}
	}
	return nil
}

func (self *IdentityManager) CreateWithAuthenticator(identity *Identity, authenticator *Authenticator, ctx *change.Context) (string, string, error) {
	if identity.Id == "" {
		identity.Id = eid.New()
	}

	if authenticator.Id == "" {
		authenticator.Id = eid.New()
	}

	if authenticator.IdentityId == "" {
		authenticator.IdentityId = identity.Id
	}

	identityType, err := self.env.GetManagers().IdentityType.ReadByIdOrName(identity.IdentityTypeId)

	if err != nil && !boltz.IsErrNotFoundErr(err) {
		return "", "", err
	}

	if identityType == nil {
		apiErr := errorz.NewNotFound()
		apiErr.Cause = errorz.NewFieldError("typeId not found", "typeId", identity.IdentityTypeId)
		apiErr.AppendCause = true
		return "", "", apiErr
	}

	err = self.env.GetDb().Update(ctx.NewMutateContext(), func(ctx boltz.MutateContext) error {
		boltIdentity, err := identity.toBoltEntityForCreate(ctx.Tx(), self.env)

		if err != nil {
			return err
		}

		if err = self.env.GetStores().Identity.Create(ctx, boltIdentity); err != nil {
			return err
		}

		boltAuthenticator, err := authenticator.toBoltEntityForCreate(ctx.Tx(), self.env)

		if err != nil {
			return err
		}

		if err = self.env.GetStores().Authenticator.Create(ctx, boltAuthenticator); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return "", "", err
	}

	return identity.Id, authenticator.Id, nil
}

func (self *IdentityManager) AssignServiceConfigs(id string, serviceConfigs []ServiceConfig, ctx *change.Context) error {
	cmd := &UpdateServiceConfigsCmd{
		manager:        self,
		identityId:     id,
		add:            true,
		serviceConfigs: serviceConfigs,
		ctx:            ctx,
	}
	return self.Dispatch(cmd)
}

func (self *IdentityManager) RemoveServiceConfigs(id string, serviceConfigs []ServiceConfig, ctx *change.Context) error {
	cmd := &UpdateServiceConfigsCmd{
		manager:        self,
		identityId:     id,
		add:            false,
		serviceConfigs: serviceConfigs,
		ctx:            ctx,
	}
	return self.Dispatch(cmd)
}

func (self *IdentityManager) ApplyUpdateServiceConfigs(cmd *UpdateServiceConfigsCmd, ctx boltz.MutateContext) error {
	identityStore := self.env.GetStores().Identity
	return self.GetDb().Update(ctx, func(ctx boltz.MutateContext) error {
		identity, err := identityStore.LoadById(ctx.Tx(), cmd.identityId)
		if err != nil {
			return err
		}

		for _, serviceConfig := range cmd.serviceConfigs {
			config, err := self.env.GetStores().Config.LoadById(ctx.Tx(), serviceConfig.Config)
			if err != nil {
				return err
			}
			if cmd.add {
				if identity.ServiceConfigs == nil {
					identity.ServiceConfigs = map[string]map[string]string{}
				}
				serviceMap, ok := identity.ServiceConfigs[serviceConfig.Service]
				if !ok {
					serviceMap = map[string]string{}
					identity.ServiceConfigs[serviceConfig.Service] = serviceMap
				}
				serviceMap[config.Type] = config.Id
			} else if identity.ServiceConfigs != nil {
				if serviceMap, ok := identity.ServiceConfigs[serviceConfig.Service]; ok {
					delete(serviceMap, config.Type)
				}
			}
		}

		return identityStore.Update(ctx, identity, boltz.MapFieldChecker{
			db.FieldIdentityServiceConfigs: struct{}{},
		})
	})
}

func (self *IdentityManager) QueryRoleAttributes(queryString string) ([]string, *models.QueryMetaData, error) {
	index := self.env.GetStores().Identity.GetRoleAttributesIndex()
	return self.queryRoleAttributes(index, queryString)
}

func (self *IdentityManager) PatchInfo(identity *Identity, changeCtx *change.Context) error {
	start := time.Now()
	checker := boltz.MapFieldChecker{
		db.FieldIdentityEnvInfoArch:       struct{}{},
		db.FieldIdentityEnvInfoOs:         struct{}{},
		db.FieldIdentityEnvInfoOsRelease:  struct{}{},
		db.FieldIdentityEnvInfoOsVersion:  struct{}{},
		db.FieldIdentityEnvInfoDomain:     struct{}{},
		db.FieldIdentityEnvInfoHostname:   struct{}{},
		db.FieldIdentitySdkInfoBranch:     struct{}{},
		db.FieldIdentitySdkInfoRevision:   struct{}{},
		db.FieldIdentitySdkInfoType:       struct{}{},
		db.FieldIdentitySdkInfoVersion:    struct{}{},
		db.FieldIdentitySdkInfoAppId:      struct{}{},
		db.FieldIdentitySdkInfoAppVersion: struct{}{},
	}

	err := self.updateEntityBatch(identity, checker, changeCtx)

	self.updateSdkInfoTimer.UpdateSince(start)

	return err
}

func (self *IdentityManager) GetConnectionTracker() *ConnectionTracker {
	return self.connections
}

// SetHasErConnection will register an identity as having an ER connection. The registration has a TTL depending on
// how the status map was configured.
func (self *IdentityManager) SetHasErConnection(identityId string) {
	self.identityStatusMap.SetHasEdgeRouterConnection(identityId)
}

// HasErConnection will return true if the supplied identity id has a current an active ER connection registered.
func (self *IdentityManager) HasErConnection(id string) bool {
	if self.statusSource == config.IdentityStatusSourceConnectEvents {
		return self.connections.GetIdentityOnlineState(id) == IdentityStateOnline
	}
	if self.statusSource == config.IdentityStatusSourceHeartbeats {
		return self.identityStatusMap.HasEdgeRouterConnection(id)
	}
	return self.connections.GetIdentityOnlineState(id) == IdentityStateOnline || self.identityStatusMap.HasEdgeRouterConnection(id)
}

func (self *IdentityManager) VisitIdentityAuthenticatorFingerprints(tx *bbolt.Tx, identityId string, visitor func(string) bool) (bool, error) {
	stopVisit := false
	err := self.visitAuthenticators(tx, identityId, func(authenticator *Authenticator) bool {
		for _, authPrint := range authenticator.Fingerprints() {
			if visitor(authPrint) {
				stopVisit = true
				return true
			}
		}
		return false
	})
	return stopVisit, err
}

func (self *IdentityManager) ReadByExternalId(externalId string) (*Identity, error) {
	query := fmt.Sprintf("%s = \"%v\"", db.FieldIdentityExternalId, externalId)

	entity, err := self.readEntityByQuery(query)

	if err != nil {
		return nil, err
	}

	if entity == nil {
		return nil, nil
	}

	identity, ok := entity.(*Identity)

	if !ok {
		return nil, fmt.Errorf("could not cast from %T to %T", entity, identity)
	}

	return identity, nil
}

func (self *IdentityManager) Disable(identityId string, duration time.Duration, ctx *change.Context) error {
	if duration < 0 {
		duration = 0
	}

	fieldMap := fields.UpdatedFieldsMap{
		db.FieldIdentityDisabledAt:    struct{}{},
		db.FieldIdentityDisabledUntil: struct{}{},
	}

	lockedAt := time.Now()
	var lockedUntil *time.Time

	if duration != 0 {
		until := lockedAt.Add(duration)
		lockedUntil = &until
	}

	err := self.Update(&Identity{
		BaseEntity: models.BaseEntity{
			Id: identityId,
		},
		DisabledAt:    &lockedAt,
		DisabledUntil: lockedUntil,
	}, fieldMap, ctx)

	if err != nil {
		return err
	}

	return self.GetEnv().GetManagers().ApiSession.DeleteByIdentityId(identityId, ctx)
}

func (self *IdentityManager) Enable(identityId string, ctx *change.Context) error {
	fieldMap := fields.UpdatedFieldsMap{
		db.FieldIdentityDisabledAt:    struct{}{},
		db.FieldIdentityDisabledUntil: struct{}{},
	}

	return self.Update(&Identity{
		BaseEntity: models.BaseEntity{
			Id: identityId,
		},
		DisabledAt:    nil,
		DisabledUntil: nil,
	}, fieldMap, ctx)
}

func (self *IdentityManager) GetIdentityStatusMapCopy() map[string]map[string]channel.Channel {
	result := map[string]map[string]channel.Channel{}
	for entry := range self.connections.connections.IterBuffered() {
		routerMap := map[string]channel.Channel{}
		entry.Val.Lock()
		for routerId, ch := range entry.Val.routers {
			routerMap[routerId] = ch
		}
		entry.Val.Unlock()
		result[entry.Key] = routerMap
	}
	return result
}

func (self *IdentityManager) IdentityToProtobuf(entity *Identity) (*edge_cmd_pb.Identity, error) {
	tags, err := edge_cmd_pb.EncodeTags(entity.Tags)
	if err != nil {
		return nil, err
	}

	var envInfo *edge_cmd_pb.Identity_EnvInfo
	if entity.EnvInfo != nil {
		envInfo = &edge_cmd_pb.Identity_EnvInfo{
			Arch:      entity.EnvInfo.Arch,
			Os:        entity.EnvInfo.Os,
			OsRelease: entity.EnvInfo.OsRelease,
			OsVersion: entity.EnvInfo.OsVersion,
			Domain:    entity.EnvInfo.Domain,
			Hostname:  entity.EnvInfo.Hostname,
		}
	}

	var sdkInfo *edge_cmd_pb.Identity_SdkInfo
	if entity.SdkInfo != nil {
		sdkInfo = &edge_cmd_pb.Identity_SdkInfo{
			AppId:      entity.SdkInfo.AppId,
			AppVersion: entity.SdkInfo.AppVersion,
			Branch:     entity.SdkInfo.Branch,
			Revision:   entity.SdkInfo.Revision,
			Type:       entity.SdkInfo.Type,
			Version:    entity.SdkInfo.Version,
		}
	}

	precedenceMap := map[string]uint32{}
	for k, v := range entity.ServiceHostingPrecedences {
		precedenceMap[k] = uint32(v)
	}

	costMap := map[string]uint32{}
	for k, v := range entity.ServiceHostingCosts {
		costMap[k] = uint32(v)
	}

	appData, err := json.Marshal(entity.AppData)
	if err != nil {
		return nil, err
	}

	msg := &edge_cmd_pb.Identity{
		Id:                        entity.Id,
		Name:                      entity.Name,
		Tags:                      tags,
		IdentityTypeId:            entity.IdentityTypeId,
		IsDefaultAdmin:            entity.IsDefaultAdmin,
		IsAdmin:                   entity.IsAdmin,
		RoleAttributes:            entity.RoleAttributes,
		EnvInfo:                   envInfo,
		SdkInfo:                   sdkInfo,
		DefaultHostingPrecedence:  uint32(entity.DefaultHostingPrecedence),
		DefaultHostingCost:        uint32(entity.DefaultHostingCost),
		ServiceHostingPrecedences: precedenceMap,
		ServiceHostingCosts:       costMap,
		AppData:                   appData,
		AuthPolicyId:              entity.AuthPolicyId,
		ExternalId:                entity.ExternalId,
		Disabled:                  entity.Disabled,
		DisabledAt:                timePtrToPb(entity.DisabledAt),
		DisabledUntil:             timePtrToPb(entity.DisabledUntil),
	}

	for serviceId, configInfo := range entity.ServiceConfigs {
		for configTypeId, configId := range configInfo {
			msg.ServiceConfigs = append(msg.ServiceConfigs, &edge_cmd_pb.Identity_ServiceConfig{
				ServiceId:    serviceId,
				ConfigTypeId: configTypeId,
				ConfigId:     configId,
			})
		}
	}

	return msg, nil
}

func (self *IdentityManager) Marshall(entity *Identity) ([]byte, error) {
	msg, err := self.IdentityToProtobuf(entity)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(msg)
}

func (self *IdentityManager) ProtobufToIdentity(msg *edge_cmd_pb.Identity) (*Identity, error) {
	var envInfo *EnvInfo
	if msg.EnvInfo != nil {
		envInfo = &EnvInfo{
			Arch:      msg.EnvInfo.Arch,
			Os:        msg.EnvInfo.Os,
			OsRelease: msg.EnvInfo.OsRelease,
			OsVersion: msg.EnvInfo.OsVersion,
			Domain:    msg.EnvInfo.Domain,
			Hostname:  msg.EnvInfo.Hostname,
		}
	}

	var sdkInfo *SdkInfo
	for msg.SdkInfo != nil {
		sdkInfo = &SdkInfo{
			AppId:      msg.SdkInfo.AppId,
			AppVersion: msg.SdkInfo.AppVersion,
			Branch:     msg.SdkInfo.Branch,
			Revision:   msg.SdkInfo.Revision,
			Type:       msg.SdkInfo.Type,
			Version:    msg.SdkInfo.Version,
		}
	}

	precedenceMap := map[string]ziti.Precedence{}
	for k, v := range msg.ServiceHostingPrecedences {
		precedenceMap[k] = ziti.Precedence(v)
	}

	costMap := map[string]uint16{}
	for k, v := range msg.ServiceHostingCosts {
		costMap[k] = uint16(v)
	}

	appData := map[string]interface{}{}
	if err := json.Unmarshal(msg.AppData, &appData); err != nil {
		return nil, err
	}

	var serviceConfigs map[string]map[string]string

	for _, serviceConfig := range msg.ServiceConfigs {
		if serviceConfigs == nil {
			serviceConfigs = map[string]map[string]string{}
		}

		serviceMap, ok := serviceConfigs[serviceConfig.ServiceId]
		if !ok {
			serviceMap = map[string]string{}
			serviceConfigs[serviceConfig.ServiceId] = serviceMap
		}
		serviceMap[serviceConfig.ConfigTypeId] = serviceConfig.ConfigId
	}

	return &Identity{
		BaseEntity: models.BaseEntity{
			Id:   msg.Id,
			Tags: edge_cmd_pb.DecodeTags(msg.Tags),
		},
		Name:                      msg.Name,
		IdentityTypeId:            msg.IdentityTypeId,
		IsDefaultAdmin:            msg.IsDefaultAdmin,
		IsAdmin:                   msg.IsAdmin,
		RoleAttributes:            msg.RoleAttributes,
		EnvInfo:                   envInfo,
		SdkInfo:                   sdkInfo,
		DefaultHostingPrecedence:  ziti.Precedence(msg.DefaultHostingPrecedence),
		DefaultHostingCost:        uint16(msg.DefaultHostingCost),
		ServiceHostingPrecedences: precedenceMap,
		ServiceHostingCosts:       costMap,
		AppData:                   appData,
		AuthPolicyId:              msg.AuthPolicyId,
		ExternalId:                msg.ExternalId,
		Disabled:                  msg.Disabled,
		DisabledAt:                pbTimeToTimePtr(msg.DisabledAt),
		DisabledUntil:             pbTimeToTimePtr(msg.DisabledUntil),
		ServiceConfigs:            serviceConfigs,
	}, nil
}

func (self *IdentityManager) Unmarshall(bytes []byte) (*Identity, error) {
	msg := &edge_cmd_pb.Identity{}
	if err := proto.Unmarshal(bytes, msg); err != nil {
		return nil, err
	}
	return self.ProtobufToIdentity(msg)
}

type CreateIdentityWithEnrollmentsCmd struct {
	manager     *IdentityManager
	identity    *Identity
	enrollments []*Enrollment
	ctx         *change.Context
}

func (self *CreateIdentityWithEnrollmentsCmd) Apply(ctx boltz.MutateContext) error {
	return self.manager.ApplyCreateWithEnrollments(self, ctx)
}

func (self *CreateIdentityWithEnrollmentsCmd) Encode() ([]byte, error) {
	identityMsg, err := self.manager.IdentityToProtobuf(self.identity)
	if err != nil {
		return nil, err
	}

	cmd := &edge_cmd_pb.CreateIdentityWithEnrollmentsCmd{
		Identity: identityMsg,
		Ctx:      ContextToProtobuf(self.ctx),
	}

	for _, enrollment := range self.enrollments {
		enrollmentMsg, err := self.manager.GetEnv().GetManagers().Enrollment.EnrollmentToProtobuf(enrollment)
		if err != nil {
			return nil, err
		}
		cmd.Enrollments = append(cmd.Enrollments, enrollmentMsg)
	}

	return cmd_pb.EncodeProtobuf(cmd)
}

func (self *CreateIdentityWithEnrollmentsCmd) Decode(env Env, msg *edge_cmd_pb.CreateIdentityWithEnrollmentsCmd) error {
	self.manager = env.GetManagers().Identity

	identity, err := self.manager.ProtobufToIdentity(msg.Identity)
	if err != nil {
		return err
	}
	self.identity = identity
	self.ctx = ProtobufToContext(msg.Ctx)
	for _, enrollmentMsg := range msg.Enrollments {
		enrollment, err := self.manager.GetEnv().GetManagers().Enrollment.ProtobufToEnrollment(enrollmentMsg)
		if err != nil {
			return err
		}
		self.enrollments = append(self.enrollments, enrollment)
	}

	return nil
}

func (self *CreateIdentityWithEnrollmentsCmd) GetChangeContext() *change.Context {
	return self.ctx
}

type identityStatusMap struct {
	identityIdToErConStatus cmap.ConcurrentMap[string, *status]
	initOnce                sync.Once
	activeDuration          time.Duration
}

type status struct {
	expiresAt time.Time
}

func newIdentityStatusMap(activeDuration time.Duration) *identityStatusMap {
	return &identityStatusMap{
		identityIdToErConStatus: cmap.New[*status](),
		activeDuration:          activeDuration,
	}
}

func (statusMap *identityStatusMap) SetHasEdgeRouterConnection(identityId string) {
	statusMap.initOnce.Do(statusMap.start)

	statusMap.identityIdToErConStatus.Set(identityId, &status{
		expiresAt: time.Now().Add(statusMap.activeDuration),
	})
}

func (statusMap *identityStatusMap) HasEdgeRouterConnection(identityId string) bool {
	if stat, ok := statusMap.identityIdToErConStatus.Get(identityId); ok {
		now := time.Now()
		ret := stat.expiresAt.After(now)
		pfxlog.Logger().
			WithField("identityId", identityId).
			WithField("expiresAt", stat.expiresAt).
			WithField("now", now).
			Tracef("reporting identity from active ER conn pool: timedout")
		return ret
	}

	pfxlog.Logger().
		WithField("identityId", identityId).
		Tracef("reporting identity from active ER conn pool: not found")
	return false
}

func (statusMap *identityStatusMap) start() {
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		for range ticker.C {
			var toRemove []string
			now := time.Now()
			statusMap.identityIdToErConStatus.IterCb(func(key string, stat *status) {
				if stat.expiresAt.Before(now) {
					pfxlog.Logger().
						WithField("identityId", key).
						WithField("expiresAt", stat.expiresAt).
						WithField("now", now).
						Debugf("removing identity from active ER conn pool: not found")
					toRemove = append(toRemove, key)
				}
			})

			for _, identityId := range toRemove {

				statusMap.identityIdToErConStatus.Remove(identityId)
			}
		}
	}()
}

type IdentityOnlineState uint32

func (self IdentityOnlineState) String() string {
	if self == IdentityStateOffline {
		return "offline"
	}
	if self == IdentityStateOnline {
		return "online"
	}
	return "unknown"
}

const (
	IdentityStateOffline IdentityOnlineState = 0
	IdentityStateOnline  IdentityOnlineState = 1
	IdentityStateUnknown IdentityOnlineState = 2
)

type identityConnections struct {
	sync.RWMutex
	routers           map[string]channel.Channel
	lastReportedState IdentityOnlineState
}

func (self *identityConnections) calculateState() IdentityOnlineState {
	// if any router is connected, the identity is online
	for _, router := range self.routers {
		if !router.IsClosed() {
			return IdentityStateOnline
		}
	}

	// if the identity is reported as connected to one or more routers, but they're all offline,
	// then the identity state is unknown
	if len(self.routers) > 0 {
		return IdentityStateUnknown
	}

	// if the identity has no router connections, it's off-line
	return IdentityStateOffline
}

func newConnectionTracker(env Env) *ConnectionTracker {
	result := &ConnectionTracker{
		connections:     cmap.New[*identityConnections](),
		eventDispatcher: env.GetEventDispatcher(),
		scanInterval:    env.GetConfig().Edge.IdentityStatusConfig.ScanInterval,
		unknownTimeout:  env.GetConfig().Edge.IdentityStatusConfig.UnknownTimeout,
		closeNotify:     env.GetCloseNotifyChannel(),
	}
	go result.runScanLoop()
	return result
}

type ConnectionTracker struct {
	connections     cmap.ConcurrentMap[string, *identityConnections]
	scanInterval    time.Duration
	unknownTimeout  time.Duration
	eventDispatcher event.Dispatcher
	closeNotify     <-chan struct{}
}

func (self *ConnectionTracker) runScanLoop() {
	ticker := time.NewTicker(self.scanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			self.ScanForDisconnectedRouters()
		case <-self.closeNotify:
			return
		}
	}
}

func (self *ConnectionTracker) ScanForDisconnectedRouters() {
	for entry := range self.connections.IterBuffered() {
		var toRemove []channel.Channel
		entry.Val.RLock()
		for _, routerCh := range entry.Val.routers {
			if routerCh.IsClosed() && routerCh.GetTimeSinceLastRead() > self.unknownTimeout {
				toRemove = append(toRemove, routerCh)
			}
		}
		entry.Val.RUnlock()

		for _, routerCh := range toRemove {
			self.MarkDisconnected(entry.Key, routerCh)
		}

		if len(toRemove) == 0 {
			var reportState *IdentityOnlineState

			entry.Val.Lock()
			lastReportedState := entry.Val.lastReportedState
			currentState := entry.Val.calculateState()
			if lastReportedState != currentState {
				reportState = &currentState
				lastReportedState = currentState
			}
			entry.Val.Unlock()

			if reportState != nil {
				self.SendEvent(entry.Key, *reportState)
			}
		}
	}
}

func (self *ConnectionTracker) MarkConnected(identityId string, ch channel.Channel) {
	var postUpsertCallback func()
	self.connections.Upsert(identityId, nil, func(exist bool, valueInMap *identityConnections, newValue *identityConnections) *identityConnections {
		if ch.IsClosed() {
			return valueInMap
		}

		if valueInMap == nil {
			valueInMap = &identityConnections{
				routers: map[string]channel.Channel{},
			}
		}

		valueInMap.Lock()
		oldState := valueInMap.calculateState()
		valueInMap.routers[ch.Id()] = ch
		newState := valueInMap.calculateState()
		lastReportedState := valueInMap.lastReportedState
		valueInMap.lastReportedState = newState
		valueInMap.Unlock()

		if newState != oldState || newState != lastReportedState {
			postUpsertCallback = func() {
				self.SendEvent(identityId, newState)
			}
		}
		return valueInMap
	})

	if postUpsertCallback != nil {
		postUpsertCallback()
	}
}

func (self *ConnectionTracker) MarkDisconnected(identityId string, ch channel.Channel) {
	var postUpsertCallback func()
	self.connections.Upsert(identityId, nil, func(exist bool, valueInMap *identityConnections, newValue *identityConnections) *identityConnections {
		if valueInMap == nil {
			return nil
		}

		valueInMap.Lock()
		oldState := valueInMap.calculateState()

		current := valueInMap.routers[ch.Id()]
		if current == nil || current == ch || current.IsClosed() {
			delete(valueInMap.routers, ch.Id())
		}

		newState := valueInMap.calculateState()
		lastReportedState := valueInMap.lastReportedState
		valueInMap.lastReportedState = newState
		valueInMap.Unlock()

		if newState != oldState || newState != lastReportedState {
			postUpsertCallback = func() {
				self.SendEvent(identityId, newState)
			}
		}
		return valueInMap
	})

	if postUpsertCallback != nil {
		postUpsertCallback()
	}
}

func (self *ConnectionTracker) SendEvent(identityId string, state IdentityOnlineState) {
	var eventType event.SdkEventType
	if state == IdentityStateOffline {
		eventType = event.SdkOffline
	} else if state == IdentityStateOnline {
		eventType = event.SdkOnline
	} else if state == IdentityStateUnknown {
		eventType = event.SdkStatusUnknown
	}

	self.eventDispatcher.AcceptSdkEvent(&event.SdkEvent{
		Namespace:  event.SdkEventsNs,
		EventType:  eventType,
		Timestamp:  time.Now(),
		IdentityId: identityId,
	})
}

func (self *ConnectionTracker) GetIdentityOnlineState(identityId string) IdentityOnlineState {
	val, _ := self.connections.Get(identityId)
	if val == nil {
		return IdentityStateOffline
	}
	val.RLock()
	defer val.RUnlock()
	return val.calculateState()
}

func (self *ConnectionTracker) SyncAllFromRouter(state *edge_ctrl_pb.ConnectEvents, ch channel.Channel) {
	m := map[string]bool{}
	for _, identityState := range state.Events {
		m[identityState.IdentityId] = identityState.IsConnected
	}

	for _, identityId := range self.connections.Keys() {
		if connected := m[identityId]; connected {
			self.MarkConnected(identityId, ch)
		} else {
			self.MarkDisconnected(identityId, ch)
		}
	}
}

func (self *ConnectionTracker) Inspect() *inspect.CtrlIdentityConnections {
	result := &inspect.CtrlIdentityConnections{
		Connections: map[string]*inspect.CtrlIdentityConnectionDetail{},
	}

	for entry := range self.connections.IterBuffered() {
		entry.Val.Lock()
		val := &inspect.CtrlIdentityConnectionDetail{
			ConnectedRouters:  map[string]*inspect.CtrlRouterConnection{},
			LastReportedState: entry.Val.lastReportedState.String(),
		}
		result.Connections[entry.Key] = val
		for routerId, ch := range entry.Val.routers {
			val.ConnectedRouters[routerId] = &inspect.CtrlRouterConnection{
				RouterId:           ch.Id(),
				Closed:             ch.IsClosed(),
				TimeSinceLastWrite: ch.GetTimeSinceLastRead().String(),
			}
		}
		entry.Val.Unlock()
	}

	return result
}

type UpdateServiceConfigsCmd struct {
	manager        *IdentityManager
	identityId     string
	add            bool
	serviceConfigs []ServiceConfig
	ctx            *change.Context
}

func (self *UpdateServiceConfigsCmd) Apply(ctx boltz.MutateContext) error {
	return self.manager.ApplyUpdateServiceConfigs(self, ctx)
}

func (self *UpdateServiceConfigsCmd) Encode() ([]byte, error) {
	cmd := &edge_cmd_pb.UpdateServiceConfigsCmd{
		IdentityId: self.identityId,
		Add:        self.add,
		Ctx:        ContextToProtobuf(self.ctx),
	}

	for _, serviceConfig := range self.serviceConfigs {
		cmd.ServiceConfigs = append(cmd.ServiceConfigs, &edge_cmd_pb.UpdateServiceConfigsCmd_ServiceConfig{
			ServiceId: serviceConfig.Service,
			ConfigId:  serviceConfig.Config,
		})
	}

	return cmd_pb.EncodeProtobuf(cmd)
}

func (self *UpdateServiceConfigsCmd) Decode(env Env, msg *edge_cmd_pb.UpdateServiceConfigsCmd) error {
	self.manager = env.GetManagers().Identity

	self.identityId = msg.IdentityId
	self.add = msg.Add
	self.ctx = ProtobufToContext(msg.Ctx)

	for _, serviceConfig := range msg.ServiceConfigs {
		self.serviceConfigs = append(self.serviceConfigs, ServiceConfig{
			Service: serviceConfig.ServiceId,
			Config:  serviceConfig.ConfigId,
		})
	}

	return nil
}

func (self *UpdateServiceConfigsCmd) GetChangeContext() *change.Context {
	return self.ctx
}
