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
	"errors"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/eid"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/metrics"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/openziti/foundation/util/errorz"
	cmap "github.com/orcaman/concurrent-map"
	"go.etcd.io/bbolt"
	"sync"
	"time"
)

const (
	IdentityActiveIntervalSeconds = 60
)

type IdentityHandler struct {
	baseHandler
	updateSdkInfoTimer metrics.Timer
	identityStatusMap  *identityStatusMap
}

func NewIdentityHandler(env Env) *IdentityHandler {
	handler := &IdentityHandler{
		baseHandler:        newBaseHandler(env, env.GetStores().Identity),
		updateSdkInfoTimer: env.GetMetricsRegistry().Timer("identity.update-sdk-info"),
		identityStatusMap:  newIdentityStatusMap(IdentityActiveIntervalSeconds * time.Second),
	}
	handler.impl = handler
	return handler
}

func (handler IdentityHandler) newModelEntity() boltEntitySink {
	return &Identity{}
}

func (handler *IdentityHandler) Create(identityModel *Identity) (string, error) {
	identityType, err := handler.env.GetHandlers().IdentityType.ReadByIdOrName(identityModel.IdentityTypeId)

	if err != nil && !boltz.IsErrNotFoundErr(err) {
		return "", err
	}

	if identityType == nil {
		apiErr := errorz.NewNotFound()
		apiErr.Cause = errorz.NewFieldError("typeId not found", "typeId", identityModel.IdentityTypeId)
		apiErr.AppendCause = true
		return "", apiErr
	}

	if identityType.Name == persistence.RouterIdentityType {
		fieldErr := errorz.NewFieldError("may not create identities with given typeId", "typeId", identityModel.IdentityTypeId)
		return "", errorz.NewFieldApiError(fieldErr)
	}

	identityModel.IdentityTypeId = identityType.Id

	return handler.createEntity(identityModel)
}

func (handler *IdentityHandler) CreateWithEnrollments(identityModel *Identity, enrollmentsModels []*Enrollment) (string, []string, error) {
	identityType, err := handler.env.GetHandlers().IdentityType.ReadByIdOrName(identityModel.IdentityTypeId)

	if err != nil && !boltz.IsErrNotFoundErr(err) {
		return "", nil, err
	}

	if identityType == nil {
		apiErr := errorz.NewNotFound()
		apiErr.Cause = errorz.NewFieldError("identityTypeId not found", "identityTypeId", identityModel.IdentityTypeId)
		apiErr.AppendCause = true
		return "", nil, apiErr
	}

	identityModel.IdentityTypeId = identityType.Id

	if identityModel.Id == "" {
		identityModel.Id = eid.New()
	}
	var enrollmentIds []string

	err = handler.GetDb().Update(func(tx *bbolt.Tx) error {
		ctx := boltz.NewMutateContext(tx)
		boltEntity, err := identityModel.toBoltEntityForCreate(tx, handler.impl)
		if err != nil {
			return err
		}
		if err := handler.GetStore().Create(ctx, boltEntity); err != nil {
			pfxlog.Logger().WithError(err).Errorf("could not create %v in bolt storage", handler.GetStore().GetSingularEntityType())
			return err
		}

		for _, enrollmentModel := range enrollmentsModels {
			enrollmentModel.IdentityId = &identityModel.Id

			err := enrollmentModel.FillJwtInfo(handler.env, identityModel.Id)

			if err != nil {
				return err
			}

			enrollmentId, err := handler.env.GetHandlers().Enrollment.createEntityInTx(ctx, enrollmentModel)

			if err != nil {
				return err
			}

			enrollmentIds = append(enrollmentIds, enrollmentId)
		}
		return nil
	})

	if err != nil {
		return "", nil, err
	}

	return identityModel.Id, enrollmentIds, nil
}

func (handler *IdentityHandler) Update(identity *Identity) error {
	identityType, err := handler.env.GetHandlers().IdentityType.ReadByIdOrName(identity.IdentityTypeId)

	if err != nil && !boltz.IsErrNotFoundErr(err) {
		return err
	}

	if identityType == nil {
		apiErr := errorz.NewNotFound()
		apiErr.Cause = errorz.NewFieldError("identityTypeId not found", "identityTypeId", identity.IdentityTypeId)
		apiErr.AppendCause = true
		return apiErr
	}

	identity.IdentityTypeId = identityType.Id

	return handler.updateEntity(identity, handler)
}

func (handler *IdentityHandler) Patch(identity *Identity, checker boltz.FieldChecker) error {
	combinedChecker := &AndFieldChecker{first: handler, second: checker}
	if checker.IsUpdated("type") {
		identityType, err := handler.env.GetHandlers().IdentityType.ReadByIdOrName(identity.IdentityTypeId)
		if err != nil && !boltz.IsErrNotFoundErr(err) {
			return err
		}

		if identityType == nil {
			apiErr := errorz.NewNotFound()
			apiErr.Cause = errorz.NewFieldError("identityTypeId not found", "identityTypeId", identity.IdentityTypeId)
			apiErr.AppendCause = true
			return apiErr
		}

		identity.IdentityTypeId = identityType.Id
	}

	return handler.patchEntity(identity, combinedChecker)
}

func (handler *IdentityHandler) Delete(id string) error {
	identity, err := handler.Read(id)

	if err != nil {
		return nil
	}

	if identity.IsDefaultAdmin {
		return errorz.NewEntityCanNotBeDeleted()
	}

	return handler.deleteEntity(id)
}

func (handler IdentityHandler) IsUpdated(field string) bool {
	return field != persistence.FieldIdentityAuthenticators && field != persistence.FieldIdentityEnrollments && field != persistence.FieldIdentityIsDefaultAdmin
}

func (handler *IdentityHandler) Read(id string) (*Identity, error) {
	entity := &Identity{}
	if err := handler.readEntity(id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (handler *IdentityHandler) ReadByName(name string) (*Identity, error) {
	entity := &Identity{}
	nameIndex := handler.env.GetStores().Identity.GetNameIndex()
	if err := handler.readEntityWithIndex("name", []byte(name), nameIndex, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (handler *IdentityHandler) readInTx(tx *bbolt.Tx, id string) (*Identity, error) {
	identity := &Identity{}
	if err := handler.readEntityInTx(tx, id, identity); err != nil {
		return nil, err
	}
	return identity, nil
}

func (handler *IdentityHandler) ReadDefaultAdmin() (*Identity, error) {
	return handler.ReadOneByQuery("isDefaultAdmin = true")
}

func (handler *IdentityHandler) ReadOneByQuery(query string) (*Identity, error) {
	result, err := handler.readEntityByQuery(query)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return result.(*Identity), nil
}

func (handler *IdentityHandler) InitializeDefaultAdmin(username, password, name string) error {
	identity, err := handler.ReadDefaultAdmin()

	if err != nil && !boltz.IsErrNotFoundErr(err) {
		pfxlog.Logger().Panic(err)
	}

	if identity != nil {
		return errors.New("already initialized: Ziti Edge default admin already defined")
	}

	identityType, err := handler.env.GetHandlers().IdentityType.ReadByName(IdentityTypeUser)

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

	if _, err := handler.Create(defaultAdmin); err != nil {
		return err
	}

	if _, err := handler.env.GetHandlers().Authenticator.Create(authenticator); err != nil {
		return err
	}

	return nil
}

func (handler *IdentityHandler) CollectAuthenticators(id string, collector func(entity *Authenticator) error) error {
	return handler.GetDb().View(func(tx *bbolt.Tx) error {
		_, err := handler.readInTx(tx, id)
		if err != nil {
			return err
		}
		authenticatorIds := handler.GetStore().GetRelatedEntitiesIdList(tx, id, persistence.FieldIdentityAuthenticators)
		for _, authenticatorId := range authenticatorIds {
			authenticator := &Authenticator{}
			err := handler.env.GetHandlers().Authenticator.readEntityInTx(tx, authenticatorId, authenticator)
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

func (handler *IdentityHandler) visitAuthenticators(tx *bbolt.Tx, id string, visitor func(entity *Authenticator) bool) error {
	_, err := handler.readInTx(tx, id)
	if err != nil {
		return err
	}
	authenticatorIds := handler.GetStore().GetRelatedEntitiesIdList(tx, id, persistence.FieldIdentityAuthenticators)
	for _, authenticatorId := range authenticatorIds {
		authenticator := &Authenticator{}
		if err := handler.env.GetHandlers().Authenticator.readEntityInTx(tx, authenticatorId, authenticator); err != nil {
			return err
		}
		if visitor(authenticator) {
			return nil
		}
	}
	return nil

}

func (handler *IdentityHandler) CollectEnrollments(id string, collector func(entity *Enrollment) error) error {
	return handler.GetDb().View(func(tx *bbolt.Tx) error {
		return handler.collectEnrollmentsInTx(tx, id, collector)
	})
}

func (handler *IdentityHandler) collectEnrollmentsInTx(tx *bbolt.Tx, id string, collector func(entity *Enrollment) error) error {
	_, err := handler.readInTx(tx, id)
	if err != nil {
		return err
	}

	associationIds := handler.GetStore().GetRelatedEntitiesIdList(tx, id, persistence.FieldIdentityEnrollments)
	for _, enrollmentId := range associationIds {
		enrollment, err := handler.env.GetHandlers().Enrollment.readInTx(tx, enrollmentId)
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

func (handler *IdentityHandler) CreateWithAuthenticator(identity *Identity, authenticator *Authenticator) (string, string, error) {
	if identity.Id == "" {
		identity.Id = eid.New()
	}

	if authenticator.Id == "" {
		authenticator.Id = eid.New()
	}

	if authenticator.IdentityId == "" {
		authenticator.IdentityId = identity.Id
	}

	identityType, err := handler.env.GetHandlers().IdentityType.ReadByIdOrName(identity.IdentityTypeId)

	if err != nil && !boltz.IsErrNotFoundErr(err) {
		return "", "", err
	}

	if identityType == nil {
		apiErr := errorz.NewNotFound()
		apiErr.Cause = errorz.NewFieldError("typeId not found", "typeId", identity.IdentityTypeId)
		apiErr.AppendCause = true
		return "", "", apiErr
	}

	err = handler.env.GetDbProvider().GetDb().Update(func(tx *bbolt.Tx) error {
		ctx := boltz.NewMutateContext(tx)
		boltIdentity, err := identity.toBoltEntityForCreate(tx, handler)

		if err != nil {
			return err
		}

		if err = handler.env.GetStores().Identity.Create(ctx, boltIdentity); err != nil {
			return err
		}

		boltAuthenticator, err := authenticator.toBoltEntityForCreate(tx, handler.env.GetHandlers().Authenticator)

		if err != nil {
			return err
		}

		if err = handler.env.GetStores().Authenticator.Create(ctx, boltAuthenticator); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return "", "", err
	}

	return identity.Id, authenticator.Id, nil
}

func (handler *IdentityHandler) GetServiceConfigs(id string) ([]ServiceConfig, error) {
	var result []ServiceConfig
	err := handler.GetDb().Update(func(tx *bbolt.Tx) error {
		configs, err := handler.env.GetStores().Identity.GetServiceConfigs(tx, id)
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

func (handler *IdentityHandler) AssignServiceConfigs(id string, serviceConfigs []ServiceConfig) error {
	return handler.GetDb().Update(func(tx *bbolt.Tx) error {
		boltServiceConfigs, err := toBoltServiceConfigs(tx, handler, serviceConfigs)
		if err != nil {
			return err
		}
		return handler.env.GetStores().Identity.AssignServiceConfigs(tx, id, boltServiceConfigs...)
	})
}

func (handler *IdentityHandler) RemoveServiceConfigs(id string, serviceConfigs []ServiceConfig) error {
	return handler.GetDb().Update(func(tx *bbolt.Tx) error {
		boltServiceConfigs, err := toBoltServiceConfigs(tx, handler, serviceConfigs)
		if err != nil {
			return err
		}
		return handler.env.GetStores().Identity.RemoveServiceConfigs(tx, id, boltServiceConfigs...)
	})
}

func (handler *IdentityHandler) QueryRoleAttributes(queryString string) ([]string, *models.QueryMetaData, error) {
	index := handler.env.GetStores().Identity.GetRoleAttributesIndex()
	return handler.queryRoleAttributes(index, queryString)
}

func (handler *IdentityHandler) PatchInfo(identity *Identity) error {
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

	err := handler.patchEntityBatch(identity, checker)

	handler.updateSdkInfoTimer.UpdateSince(start)

	return err
}

func (handler *IdentityHandler) SetActive(id string) {
	handler.identityStatusMap.SetActive(id)
}

func (handler *IdentityHandler) IsActive(id string) bool {
	return handler.identityStatusMap.IsActive(id)
}

func (handler *IdentityHandler) VisitIdentityAuthenticatorFingerprints(tx *bbolt.Tx, identityId string, visitor func(string) bool) (bool, error) {
	stopVisit := false
	err := handler.visitAuthenticators(tx, identityId, func(authenticator *Authenticator) bool {
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

func (handler IdentityHandler) ReadByExternalId(externalId string) (*Identity, error) {
	query := fmt.Sprintf("%s = \"%v\"", persistence.FieldIdentityExternalId, externalId)

	entity, err := handler.readEntityByQuery(query)

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

func (handler *IdentityHandler) Disable(identityId string, duration time.Duration) error {
	if duration < 0 {
		duration = 0
	}

	fieldMap := boltz.MapFieldChecker{
		persistence.FieldIdentityDisabledAt:    struct{}{},
		persistence.FieldIdentityDisabledUntil: struct{}{},
	}

	lockedAt := time.Now()
	var lockedUntil *time.Time

	if duration != 0 {
		until := lockedAt.Add(duration)
		lockedUntil = &until
	}

	err := handler.Patch(&Identity{
		BaseEntity: models.BaseEntity{
			Id: identityId,
		},
		DisabledAt:    &lockedAt,
		DisabledUntil: lockedUntil,
	}, fieldMap)

	if err != nil {
		return err
	}

	return handler.GetEnv().GetHandlers().ApiSession.DeleteByIdentityId(identityId)
}

func (handler *IdentityHandler) Enable(identityId string) error {
	fieldMap := boltz.MapFieldChecker{
		persistence.FieldIdentityDisabledAt:    struct{}{},
		persistence.FieldIdentityDisabledUntil: struct{}{},
	}

	return handler.Patch(&Identity{
		BaseEntity: models.BaseEntity{
			Id: identityId,
		},
		DisabledAt:    nil,
		DisabledUntil: nil,
	}, fieldMap)
}

type identityStatusMap struct {
	identities     cmap.ConcurrentMap // identityId -> status{expiresAt}
	initOnce       sync.Once
	activeDuration time.Duration
}

type status struct {
	expiresAt time.Time
}

func newIdentityStatusMap(activeDuration time.Duration) *identityStatusMap {
	return &identityStatusMap{
		identities:     cmap.New(),
		activeDuration: activeDuration,
	}
}

func (statusMap *identityStatusMap) SetActive(identityId string) {
	statusMap.initOnce.Do(statusMap.start)

	statusMap.identities.Set(identityId, &status{
		expiresAt: time.Now().Add(statusMap.activeDuration),
	})
}

func (statusMap *identityStatusMap) IsActive(identityId string) bool {
	if val, ok := statusMap.identities.Get(identityId); ok {
		if status, ok := val.(*status); ok {
			return status.expiresAt.After(time.Now())
		}
	}
	return false
}

func (statusMap *identityStatusMap) start() {
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		for {
			select {
			case <-ticker.C:
				var toRemove []string
				now := time.Now()
				statusMap.identities.IterCb(func(key string, val interface{}) {
					if status, ok := val.(*status); !ok || status.expiresAt.Before(now) {
						toRemove = append(toRemove, key)
					}
				})

				for _, identityId := range toRemove {
					statusMap.identities.Remove(identityId)
				}
			}
		}
	}()
}
