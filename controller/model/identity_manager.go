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
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/eid"
	"github.com/openziti/edge/pb/edge_cmd_pb"
	"github.com/openziti/fabric/controller/command"
	"github.com/openziti/fabric/controller/fields"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/pb/cmd_pb"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/metrics"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/storage/boltz"
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
	baseEntityManager
	updateSdkInfoTimer metrics.Timer
	identityStatusMap  *identityStatusMap
}

func NewIdentityManager(env Env) *IdentityManager {
	manager := &IdentityManager{
		baseEntityManager:  newBaseEntityManager(env, env.GetStores().Identity),
		updateSdkInfoTimer: env.GetMetricsRegistry().Timer("identity.update-sdk-info"),
		identityStatusMap:  newIdentityStatusMap(IdentityActiveIntervalSeconds * time.Second),
	}
	manager.impl = manager

	network.RegisterManagerDecoder[*Identity](env.GetHostController().GetNetwork().GetManagers(), manager)
	RegisterCommand(env, &CreateIdentityWithEnrollmentsCmd{}, &edge_cmd_pb.CreateIdentityWithEnrollmentsCmd{})

	return manager
}

func (self *IdentityManager) newModelEntity() edgeEntity {
	return &Identity{}
}

func (self *IdentityManager) Create(entity *Identity) error {
	return network.DispatchCreate[*Identity](self, entity)
}

func (self *IdentityManager) ApplyCreate(cmd *command.CreateEntityCommand[*Identity]) error {
	_, err := self.createEntity(cmd.Entity)
	return err
}

func (self *IdentityManager) CreateWithEnrollments(identityModel *Identity, enrollmentsModels []*Enrollment) error {
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
	}

	return self.Dispatch(cmd)
}

func (self *IdentityManager) ApplyCreateWithEnrollments(cmd *CreateIdentityWithEnrollmentsCmd) error {
	identityModel := cmd.identity
	enrollmentsModels := cmd.enrollments

	return self.GetDb().Update(func(tx *bbolt.Tx) error {
		ctx := boltz.NewMutateContext(tx)
		boltEntity, err := identityModel.toBoltEntityForCreate(tx, self.impl)
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

func (self *IdentityManager) Update(entity *Identity, checker fields.UpdatedFields) error {
	return network.DispatchUpdate[*Identity](self, entity, checker)
}

func (self *IdentityManager) ApplyUpdate(cmd *command.UpdateEntityCommand[*Identity]) error {
	var checker boltz.FieldChecker = self
	if cmd.UpdatedFields != nil {
		checker = &AndFieldChecker{first: self, second: cmd.UpdatedFields}
	}
	return self.updateEntity(cmd.Entity, checker)
}

func (self *IdentityManager) IsUpdated(field string) bool {
	return field != persistence.FieldIdentityAuthenticators && field != persistence.FieldIdentityEnrollments && field != persistence.FieldIdentityIsDefaultAdmin
}

func (self *IdentityManager) Read(id string) (*Identity, error) {
	entity := &Identity{}
	if err := self.readEntity(id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (self *IdentityManager) ReadByName(name string) (*Identity, error) {
	entity := &Identity{}
	nameIndex := self.env.GetStores().Identity.GetNameIndex()
	if err := self.readEntityWithIndex("name", []byte(name), nameIndex, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (self *IdentityManager) readInTx(tx *bbolt.Tx, id string) (*Identity, error) {
	identity := &Identity{}
	if err := self.readEntityInTx(tx, id, identity); err != nil {
		return nil, err
	}
	return identity, nil
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
	identity, err := self.ReadDefaultAdmin()

	if err != nil && !boltz.IsErrNotFoundErr(err) {
		return err
	}

	if identity != nil {
		return errors.New("already initialized: Ziti Edge default admin already defined")
	}

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

	identityType, err := self.env.GetManagers().IdentityType.ReadByName(IdentityTypeUser)

	if err != nil {
		return err
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
		Method:     persistence.MethodAuthenticatorUpdb,
		IdentityId: identityId,
		SubType: &AuthenticatorUpdb{
			Username: username,
			Password: password,
		},
	}

	if err = self.Create(defaultAdmin); err != nil {
		return err
	}

	if err = self.env.GetManagers().Authenticator.Create(authenticator); err != nil {
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
		authenticatorIds := self.GetStore().GetRelatedEntitiesIdList(tx, id, persistence.FieldIdentityAuthenticators)
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
	authenticatorIds := self.GetStore().GetRelatedEntitiesIdList(tx, id, persistence.FieldIdentityAuthenticators)
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

	associationIds := self.GetStore().GetRelatedEntitiesIdList(tx, id, persistence.FieldIdentityEnrollments)
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

func (self *IdentityManager) CreateWithAuthenticator(identity *Identity, authenticator *Authenticator) (string, string, error) {
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

	err = self.env.GetDbProvider().GetDb().Update(func(tx *bbolt.Tx) error {
		ctx := boltz.NewMutateContext(tx)
		boltIdentity, err := identity.toBoltEntityForCreate(tx, self)

		if err != nil {
			return err
		}

		if err = self.env.GetStores().Identity.Create(ctx, boltIdentity); err != nil {
			return err
		}

		boltAuthenticator, err := authenticator.toBoltEntityForCreate(tx, self.env.GetManagers().Authenticator)

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

func (self *IdentityManager) GetServiceConfigs(id string) ([]ServiceConfig, error) {
	var result []ServiceConfig
	err := self.GetDb().Update(func(tx *bbolt.Tx) error {
		configs, err := self.env.GetStores().Identity.GetServiceConfigs(tx, id)
		if err != nil {
			return err
		}
		for _, config := range configs {
			result = append(result, ServiceConfig{Service: config.ServiceId, Config: config.ConfigId})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (self *IdentityManager) AssignServiceConfigs(id string, serviceConfigs []ServiceConfig) error {
	return self.GetDb().Update(func(tx *bbolt.Tx) error {
		boltServiceConfigs, err := toBoltServiceConfigs(tx, self, serviceConfigs)
		if err != nil {
			return err
		}
		return self.env.GetStores().Identity.AssignServiceConfigs(tx, id, boltServiceConfigs...)
	})
}

func (self *IdentityManager) RemoveServiceConfigs(id string, serviceConfigs []ServiceConfig) error {
	return self.GetDb().Update(func(tx *bbolt.Tx) error {
		boltServiceConfigs, err := toBoltServiceConfigs(tx, self, serviceConfigs)
		if err != nil {
			return err
		}
		return self.env.GetStores().Identity.RemoveServiceConfigs(tx, id, boltServiceConfigs...)
	})
}

func (self *IdentityManager) QueryRoleAttributes(queryString string) ([]string, *models.QueryMetaData, error) {
	index := self.env.GetStores().Identity.GetRoleAttributesIndex()
	return self.queryRoleAttributes(index, queryString)
}

func (self *IdentityManager) PatchInfo(identity *Identity) error {
	start := time.Now()
	checker := boltz.MapFieldChecker{
		persistence.FieldIdentityEnvInfoArch:       struct{}{},
		persistence.FieldIdentityEnvInfoOs:         struct{}{},
		persistence.FieldIdentityEnvInfoOsRelease:  struct{}{},
		persistence.FieldIdentityEnvInfoOsVersion:  struct{}{},
		persistence.FieldIdentitySdkInfoBranch:     struct{}{},
		persistence.FieldIdentitySdkInfoRevision:   struct{}{},
		persistence.FieldIdentitySdkInfoType:       struct{}{},
		persistence.FieldIdentitySdkInfoVersion:    struct{}{},
		persistence.FieldIdentitySdkInfoAppId:      struct{}{},
		persistence.FieldIdentitySdkInfoAppVersion: struct{}{},
	}

	err := self.updateEntityBatch(identity, checker)

	self.updateSdkInfoTimer.UpdateSince(start)

	return err
}

func (self *IdentityManager) SetActive(id string) {
	self.identityStatusMap.SetActive(id)
}

func (self *IdentityManager) IsActive(id string) bool {
	return self.identityStatusMap.IsActive(id)
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
	query := fmt.Sprintf("%s = \"%v\"", persistence.FieldIdentityExternalId, externalId)

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

func (self *IdentityManager) Disable(identityId string, duration time.Duration) error {
	if duration < 0 {
		duration = 0
	}

	fieldMap := fields.UpdatedFieldsMap{
		persistence.FieldIdentityDisabledAt:    struct{}{},
		persistence.FieldIdentityDisabledUntil: struct{}{},
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
	}, fieldMap)

	if err != nil {
		return err
	}

	return self.GetEnv().GetManagers().ApiSession.DeleteByIdentityId(identityId)
}

func (self *IdentityManager) Enable(identityId string) error {
	fieldMap := fields.UpdatedFieldsMap{
		persistence.FieldIdentityDisabledAt:    struct{}{},
		persistence.FieldIdentityDisabledUntil: struct{}{},
	}

	return self.Update(&Identity{
		BaseEntity: models.BaseEntity{
			Id: identityId,
		},
		DisabledAt:    nil,
		DisabledUntil: nil,
	}, fieldMap)
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
}

func (self *CreateIdentityWithEnrollmentsCmd) Apply() error {
	return self.manager.ApplyCreateWithEnrollments(self)
}

func (self *CreateIdentityWithEnrollmentsCmd) Encode() ([]byte, error) {
	identityMsg, err := self.manager.IdentityToProtobuf(self.identity)
	if err != nil {
		return nil, err
	}

	cmd := &edge_cmd_pb.CreateIdentityWithEnrollmentsCmd{
		Identity: identityMsg,
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

	for _, enrollmentMsg := range msg.Enrollments {
		enrollment, err := self.manager.GetEnv().GetManagers().Enrollment.ProtobufToEnrollment(enrollmentMsg)
		if err != nil {
			return err
		}
		self.enrollments = append(self.enrollments, enrollment)
	}

	return nil
}

type identityStatusMap struct {
	identityIdToStatus cmap.ConcurrentMap[string, *status]
	initOnce           sync.Once
	activeDuration     time.Duration
}

type status struct {
	expiresAt time.Time
}

func newIdentityStatusMap(activeDuration time.Duration) *identityStatusMap {
	return &identityStatusMap{
		identityIdToStatus: cmap.New[*status](),
		activeDuration:     activeDuration,
	}
}

func (statusMap *identityStatusMap) SetActive(identityId string) {
	statusMap.initOnce.Do(statusMap.start)

	statusMap.identityIdToStatus.Set(identityId, &status{
		expiresAt: time.Now().Add(statusMap.activeDuration),
	})
}

func (statusMap *identityStatusMap) IsActive(identityId string) bool {
	if stat, ok := statusMap.identityIdToStatus.Get(identityId); ok {
		return stat.expiresAt.After(time.Now())
	}
	return false
}

func (statusMap *identityStatusMap) start() {
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		for range ticker.C {
			var toRemove []string
			now := time.Now()
			statusMap.identityIdToStatus.IterCb(func(key string, stat *status) {
				if stat.expiresAt.Before(now) {
					toRemove = append(toRemove, key)
				}
			})

			for _, identityId := range toRemove {
				statusMap.identityIdToStatus.Remove(identityId)
			}
		}
	}()
}
