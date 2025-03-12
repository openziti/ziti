package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/command"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/fields"
	"github.com/openziti/ziti/controller/models"
	"go.etcd.io/bbolt"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

var _ edgeEntity[*db.ControllerSetting] = (*ControllerSetting)(nil)
var _ boltz.ExtEntity = (*db.ControllerSetting)(nil)

func newControllerSettingManager(env Env) *ControllerSettingManager {
	result := &ControllerSettingManager{
		baseEntityManager: newBaseEntityManager[*ControllerSetting, *db.ControllerSetting](env, env.GetStores().ControllerSetting),
	}
	result.impl = result

	RegisterManagerDecoder[*ControllerSetting](env, result)

	return result
}

type ControllerSettingManager struct {
	baseEntityManager[*ControllerSetting, *db.ControllerSetting]
}

// ReadEffective returns a controller setting object that contains the effective settings and instance settings for a
// specific controller. Effective settings are global + instance overrides.
func (s *ControllerSettingManager) ReadEffective(id string) (*ControllerSettingEffective, error) {

	var global, instance, effective *ControllerSetting

	err := s.env.GetDb().View(func(tx *bbolt.Tx) error {
		var err error
		globalSt, err := s.Store.LoadById(tx, db.ControllerSettingGlobalId)

		if err != nil {
			return fmt.Errorf("could not read global settings: %w", err)
		}

		global = &ControllerSetting{}

		err = global.fillFrom(s.env, tx, globalSt)

		if err != nil {
			return fmt.Errorf("could not fill global settings: %w", err)
		}

		// if reading effective of global, don't do any more work, global == effective == instance
		if id == db.ControllerSettingGlobalId {
			instance = global
			effective = global
			return nil
		}

		instanceSt, err := s.Store.LoadById(tx, id)

		if err != nil && !boltz.IsErrNotFoundErr(err) {
			return fmt.Errorf("could not read instance settings: %w", err)
		}

		if instanceSt != nil {
			instance = &ControllerSetting{}
			err = instance.fillFrom(s.env, tx, instanceSt)

			effectiveSt := instanceSt.MergeSettings(globalSt)

			err = effective.fillFrom(s.env, tx, effectiveSt)

			if err != nil {
				return fmt.Errorf("could not fill effective settings: %w", err)
			}
		} else {
			effective = global
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return &ControllerSettingEffective{
		Effective: effective,
		Instance:  instance,
	}, nil
}

func (s *ControllerSettingManager) newModelEntity() *ControllerSetting {
	return &ControllerSetting{
		BaseEntity: models.BaseEntity{},
		Oidc: &OidcSettings{
			OidcSettingDef: &db.OidcSettingDef{
				RedirectUris:   nil,
				PostLogoutUris: nil,
			},
		},
	}
}

type ControllerSettingListener func(settingPath string, setting *ControllerSetting)

func (s *ControllerSettingManager) Watch(settingPath string, listener func(string, *ControllerSetting)) {
	s.env.GetStores().ControllerSetting.Watch(settingPath, func(setting string, controllerId string, settingEvent *db.ControllerSettingsEvent) {

		controllerSettingModel := &ControllerSetting{
			BaseEntity: models.BaseEntity{},
			Oidc: &OidcSettings{
				OidcSettingDef: &db.OidcSettingDef{},
			},
		}

		controllerSettingModel.BaseEntity.FillCommon(settingEvent.Effective)
		controllerSettingModel.Oidc.OidcSettingDef = settingEvent.Effective.Oidc

		listener(settingPath, controllerSettingModel)
	}, s.env.GetControllerId())
}

func (s *ControllerSettingManager) Marshall(entity *ControllerSetting) ([]byte, error) {
	jsonData, err := json.Marshal(entity)
	if err != nil {
		return nil, fmt.Errorf("could not marshal entity: %w", err)
	}

	var pbStruct structpb.Struct
	err = json.Unmarshal(jsonData, &pbStruct)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal into pbStruct: %w", err)
	}

	return proto.Marshal(&pbStruct)
}

func (s *ControllerSettingManager) Unmarshall(bytes []byte) (*ControllerSetting, error) {
	var pbStruct structpb.Struct
	err := proto.Unmarshal(bytes, &pbStruct)

	if err != nil {
		return nil, fmt.Errorf("could not unmarshal []byte into pbStruct: %w", err)
	}

	jsonStr, err := json.Marshal(&pbStruct)

	if err != nil {
		return nil, fmt.Errorf("could not marshal pbStruct to JSON: %w", err)
	}

	var setting ControllerSetting
	err = json.Unmarshal(jsonStr, &setting)

	if err != nil {
		return nil, fmt.Errorf("could not unmarshal JSON into ControllerSetting: %w", err)
	}

	return &setting, nil
}

func (s *ControllerSettingManager) ApplyCreate(cmd *command.CreateEntityCommand[*ControllerSetting], ctx boltz.MutateContext) error {
	_, err := s.createEntity(cmd.Entity, ctx)
	return err
}

func (s *ControllerSettingManager) ApplyUpdate(cmd *command.UpdateEntityCommand[*ControllerSetting], ctx boltz.MutateContext) error {
	return s.updateEntity(cmd.Entity, cmd.UpdatedFields, ctx)
}

func (s *ControllerSettingManager) Create(setting *ControllerSetting, ctx *change.Context) error {

	if setting.Id == "" {
		return errors.New("id is required")
	}

	if setting.Id == db.ControllerSettingGlobalId || setting.Id == db.ControllerSettingAny {
		return fmt.Errorf("cannot create settings for controllers with id: %s", setting.Id)
	}

	controller, err := s.env.GetManagers().Controller.Read(setting.Id)

	if err != nil || controller == nil {
		return fmt.Errorf("could not locate controller with id: %s", setting.Id)
	}

	return DispatchCreate[*ControllerSetting](s, setting, ctx)
}

func (s *ControllerSettingManager) Update(setting *ControllerSetting, checker fields.UpdatedFields, ctx *change.Context) error {

	if setting.Id == db.ControllerSettingAny {
		return fmt.Errorf("cannot update settings for controllers with id: %s", setting.Id)
	}

	return DispatchUpdate[*ControllerSetting](s, setting, checker, ctx)
}
