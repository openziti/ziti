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

package db

import (
	"context"
	"errors"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/storage/boltz"
	"go.etcd.io/bbolt"
	"reflect"
)

const (
	ControllerSettingGlobalId = "global"
	ControllerSettingAny      = "any"

	FieldControllerSettingOidc = "oidc"

	FieldControllerSettingOidcRedirectUris   = FieldControllerSettingOidc + ".redirectUris"
	FieldControllerSettingOidcPostLogoutUris = FieldControllerSettingOidc + ".postLogoutUris"
	FieldControllerSettingIsMigration        = "isMigration"
)

type controllerFieldChangeContextKey string

const controllerFieldChangeContext controllerFieldChangeContextKey = "controllerFieldChangeContext"

type ControllerSettingDef interface {
	Id() string
	FillEntity(bucket *boltz.TypedBucket)
	PersistEntity(ctx *boltz.PersistContext, bucket *boltz.TypedBucket, entity *ControllerSetting)
}

type ControllerSetting struct {
	boltz.BaseExtEntity
	Oidc *OidcSettingDef

	IsMigration bool
}

func (entity *ControllerSetting) GetEntityType() string {
	return EntityTypeSettings
}

// MergeSettings returns a new setting instance with the global settings overridden by the provided settings
func (entity *ControllerSetting) MergeSettings(setting *ControllerSetting) *ControllerSetting {
	newCs := &ControllerSetting{
		BaseExtEntity: setting.BaseExtEntity,
		Oidc:          entity.Oidc,
	}

	if setting.Oidc != nil {
		newCs.Oidc = setting.Oidc
	}

	return newCs
}

var _ ControllerSettingStore = (*controllerSettingStoreImpl)(nil)

type ControllerSettingStore interface {
	Store[*ControllerSetting]
	Watch(setting string, listener ControllerSettingListener, controllerId string, additionalControllerIds ...string)
	CreateGlobalDefault(ctx boltz.MutateContext, settings *ControllerSetting) error
}

func newControllerSettingStore(provider DbProviderF, stores *stores) *controllerSettingStoreImpl {
	store := &controllerSettingStoreImpl{
		dbProvider: provider,
	}
	store.baseStore = newBaseStore[*ControllerSetting](stores, store)
	store.InitImpl(store)
	return store
}

type controllerSettingStoreImpl struct {
	*baseStore[*ControllerSetting]
	indexName                  boltz.ReadIndex
	symbolEnrollments          boltz.EntitySetSymbol
	settingControllerListeners map[string]map[string][]ControllerSettingListener //setting path -> controller id ->  -> listeners
	dbProvider                 DbProviderF
}

func (store *controllerSettingStoreImpl) CreateGlobalDefault(ctx boltz.MutateContext, settings *ControllerSetting) error {
	settings.Id = ControllerSettingGlobalId
	settings.IsSystem = true

	_ = store.DeleteById(ctx, ControllerSettingGlobalId)

	return store.Create(ctx, settings)
}

func (store *controllerSettingStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()

	store.AddEntityConstraint(&untypedConstraintWithCtx[*ControllerSetting]{
		eventListener: store.onCreateOrUpdate,
		changeTypes:   []boltz.EntityEventType{boltz.EntityCreated, boltz.EntityUpdated},
	})

	store.AddListener(store.onDelete, boltz.EntityDeleted)
}

type untypedConstraintWithCtx[E boltz.Entity] struct {
	eventListener func(boltz.MutateContext, boltz.Entity)
	changeTypes   []boltz.EntityEventType
}

func (self *untypedConstraintWithCtx[E]) ProcessPreCommit(*boltz.EntityChangeState[E]) error {
	return nil
}

func (self *untypedConstraintWithCtx[E]) ProcessPostCommit(state *boltz.EntityChangeState[E]) {
	for _, changeType := range self.changeTypes {
		if state.ChangeType == boltz.EntityCreated && changeType.IsCreate() {
			if changeType.IsAsync() {
				go self.eventListener(state.Ctx, state.FinalState)
			} else {
				self.eventListener(state.Ctx, state.FinalState)
			}
		} else if state.ChangeType == boltz.EntityUpdated && changeType.IsUpdate() {
			if changeType.IsAsync() {
				go self.eventListener(state.Ctx, state.FinalState)
			} else {
				self.eventListener(state.Ctx, state.FinalState)
			}
		} else if state.ChangeType == boltz.EntityDeleted && changeType.IsDelete() {
			if changeType.IsAsync() {
				go self.eventListener(state.Ctx, state.InitialState)
			} else {
				self.eventListener(state.Ctx, state.InitialState)
			}
		}
	}
}

func (store *controllerSettingStoreImpl) initializeLinked() {}

func (store *controllerSettingStoreImpl) NewEntity() *ControllerSetting {
	return &ControllerSetting{
		Oidc: &OidcSettingDef{
			RedirectUris:   nil,
			PostLogoutUris: nil,
		},
	}
}

func (store *controllerSettingStoreImpl) DeleteById(ctx boltz.MutateContext, id string) error {
	policy, err := store.LoadById(ctx.Tx(), id)
	if err != nil {
		return err
	}
	if !policy.IsSystem || ctx.IsSystemContext() {
		return store.BaseStore.DeleteById(ctx, id)
	}
	return errorz.NewEntityCanNotBeDeletedFrom(errors.New("default settings cannot be removed, only updated"))
}

type ControllerSettingsEvent struct {
	Effective *ControllerSetting
	Instance  *ControllerSetting
	Global    *ControllerSetting
}

type ControllerSettingListener func(setting string, controllerId string, settingEvent *ControllerSettingsEvent)

func (store *controllerSettingStoreImpl) Watch(setting string, listener ControllerSettingListener, controllerId string, additionalControllerIds ...string) {

	additionalControllerIds = append(additionalControllerIds, controllerId)

	if store.settingControllerListeners == nil {
		store.settingControllerListeners = make(map[string]map[string][]ControllerSettingListener)
	}

	for _, id := range additionalControllerIds {
		if store.settingControllerListeners[setting] == nil {
			store.settingControllerListeners[setting] = make(map[string][]ControllerSettingListener)
		}

		if store.settingControllerListeners[setting][id] == nil {
			store.settingControllerListeners[setting][id] = make([]ControllerSettingListener, 0)
		}

		store.settingControllerListeners[setting][id] = append(store.settingControllerListeners[setting][id], listener)
	}
}

func (store *controllerSettingStoreImpl) onDelete(entity boltz.Entity) {
	setting := entity.(*ControllerSetting)

	for _, controllerListeners := range store.settingControllerListeners {
		delete(controllerListeners, setting.Id)
	}
}

func (store *controllerSettingStoreImpl) onCreateOrUpdate(ctx boltz.MutateContext, entity boltz.Entity) {
	setting := entity.(*ControllerSetting)

	changedPaths := getChangedPaths(ctx)
	pathMap := make(map[string]struct{})

	for _, path := range changedPaths {
		pathMap[path] = struct{}{}
	}

	var globalSettings *ControllerSetting
	err := store.dbProvider().View(func(tx *bbolt.Tx) error {
		var err error
		globalSettings, err = store.GetGlobalSettings(tx)
		return err
	})

	if err != nil {
		pfxlog.Logger().WithError(err).Errorf("error processing create/update settings changes, could not get global configuration settings")
		return
	}

	effectiveSettings := globalSettings.MergeSettings(setting)

	eventData := &ControllerSettingsEvent{
		Effective: effectiveSettings,
		Instance:  setting,
		Global:    globalSettings,
	}

	for path := range pathMap {
		settingControllerListeners := store.settingControllerListeners[path]

		for _, controllerListeners := range settingControllerListeners[setting.Id] {
			controllerListeners(path, setting.Id, eventData)
		}

		for _, controllerListeners := range settingControllerListeners[ControllerSettingAny] {
			controllerListeners(path, setting.Id, eventData)
		}
	}
}

func (store *controllerSettingStoreImpl) FillEntity(entity *ControllerSetting, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)

	entityVal := reflect.ValueOf(entity).Elem()
	entityType := reflect.TypeOf(entity).Elem()

	for i := 0; i < entityType.NumField(); i++ {
		field := entityVal.Field(i)
		if field.Kind() == reflect.Ptr && !field.IsNil() {
			if field.Type().Implements(reflect.TypeOf((*ControllerSettingDef)(nil)).Elem()) {
				// Cast to ControllerSettingDef and call FillEntity()
				setting := field.Interface().(ControllerSettingDef)
				settingBucketId := setting.Id()

				settingBucket := bucket.GetOrCreateBucket(settingBucketId)
				setting.FillEntity(settingBucket)
			}
		}
	}

	entity.IsMigration = bucket.GetBoolWithDefault(FieldControllerSettingIsMigration, false)
}

func (store *controllerSettingStoreImpl) PersistEntity(entity *ControllerSetting, ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)

	entityVal := reflect.ValueOf(entity).Elem()
	entityType := reflect.TypeOf(entity).Elem()

	for i := 0; i < entityType.NumField(); i++ {
		field := entityVal.Field(i)
		if field.Kind() == reflect.Ptr && !field.IsNil() {
			if field.Type().Implements(reflect.TypeOf((*ControllerSettingDef)(nil)).Elem()) {
				// Cast to ControllerSettingDef and call FillEntity()
				setting := field.Interface().(ControllerSettingDef)
				settingBucketId := setting.Id()

				settingBucket := ctx.Bucket.GetOrCreateBucket(settingBucketId)
				setting.PersistEntity(ctx, settingBucket, entity)
			}
		}
	}

	ctx.SetBool(FieldControllerSettingIsMigration, entity.IsMigration)
}

func (store *controllerSettingStoreImpl) GetGlobalSettings(tx *bbolt.Tx) (*ControllerSetting, error) {
	return store.LoadById(tx, ControllerSettingGlobalId)
}

var _ ControllerSettingDef = (*OidcSettingDef)(nil)

type OidcSettingDef struct {
	RedirectUris   []string
	PostLogoutUris []string
}

func (o *OidcSettingDef) Id() string {
	return FieldControllerSettingOidc
}

func (o *OidcSettingDef) FillEntity(bucket *boltz.TypedBucket) {
	o.RedirectUris = bucket.GetStringList(FieldControllerSettingOidcRedirectUris)
	o.PostLogoutUris = bucket.GetStringList(FieldControllerSettingOidcPostLogoutUris)
}

func getChangedPaths(ctx boltz.MutateContext) (changedPaths []string) {
	v := ctx.Context().Value(controllerFieldChangeContext)

	if v == nil {
		return nil
	}

	return v.(*PathSet).Paths
}

type PathSet struct {
	Paths []string
}

func addChangedPath(ctx boltz.MutateContext, newPaths ...string) {
	v := ctx.Context().Value(controllerFieldChangeContext)

	var changedPaths *PathSet

	if v == nil {
		changedPaths = &PathSet{}
		ctx.UpdateContext(func(ctx context.Context) context.Context {
			return context.WithValue(ctx, controllerFieldChangeContext, changedPaths)
		})
	} else {
		changedPaths = v.(*PathSet)
	}

	changedPaths.Paths = append(changedPaths.Paths, newPaths...)
}

func (o *OidcSettingDef) PersistEntity(ctx *boltz.PersistContext, bucket *boltz.TypedBucket, entity *ControllerSetting) {
	settingsChanged := false

	if ctx.FieldChecker == nil || ctx.FieldChecker.IsUpdated(FieldControllerSettingOidcRedirectUris) {
		if !stringz.EqualSlices(bucket.GetStringList(FieldControllerSettingOidcRedirectUris), o.RedirectUris) {
			bucket.SetStringList(FieldControllerSettingOidcRedirectUris, o.RedirectUris, ctx.FieldChecker)
			addChangedPath(ctx, FieldControllerSettingOidcRedirectUris)
			settingsChanged = true
		}
	}

	if ctx.FieldChecker == nil || ctx.FieldChecker.IsUpdated(FieldControllerSettingOidcPostLogoutUris) {
		if !stringz.EqualSlices(bucket.GetStringList(FieldControllerSettingOidcPostLogoutUris), o.PostLogoutUris) {
			bucket.SetStringList(FieldControllerSettingOidcPostLogoutUris, o.PostLogoutUris, ctx.FieldChecker)
			addChangedPath(ctx, FieldControllerSettingOidcPostLogoutUris)
			settingsChanged = true
		}
	}

	if settingsChanged {
		addChangedPath(ctx, FieldControllerSettingOidc)
	}
}
