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
	"fmt"
	"strings"

	"github.com/openziti/ziti/v2/common/config/routerlink"
	"github.com/openziti/ziti/v2/common/pb/edge_cmd_pb"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/command"
	"github.com/openziti/ziti/v2/controller/db"
	"github.com/openziti/ziti/v2/controller/fields"
	"github.com/openziti/ziti/v2/controller/models"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"go.etcd.io/bbolt"
	"google.golang.org/protobuf/proto"
)

// ConfigDataValidator applies rules to a config's data that its JSON schema
// can't express: relationships between fields, combinations of values, anything
// needing more than per-field checks. Registered against a config type name and
// run on create and update, after schema validation has passed.
//
// Return a field error (errorz.NewFieldError) to have the REST layer report the
// offending field; a plain error is reported against the data as a whole.
type ConfigDataValidator func(data map[string]interface{}) error

func NewConfigManager(env Env) *ConfigManager {
	manager := &ConfigManager{
		baseEntityManager: newBaseEntityManager[*Config, *db.Config](env, env.GetStores().Config),
		dataValidators:    map[string]ConfigDataValidator{},
	}
	manager.impl = manager

	RegisterManagerDecoder[*Config](env, manager)

	manager.RegisterDataValidator(routerlink.ConfigTypeV1, validateRouterLinkConfigData)

	return manager
}

// validateRouterLinkConfigData applies the router.link rules its JSON schema
// can't express, notably heartbeat timing, using the router's own validation.
func validateRouterLinkConfigData(data map[string]interface{}) error {
	js, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("unable to re-encode config data: %w", err)
	}
	cfg, err := routerlink.ParseConfig(string(js))
	if err != nil {
		return err
	}
	return cfg.Validate()
}

type ConfigManager struct {
	baseEntityManager[*Config, *db.Config]

	// dataValidators is keyed by config type name, which is stable across networks
	// where ids are not. Written only during controller init, so reads need no lock.
	dataValidators map[string]ConfigDataValidator
}

// RegisterDataValidator installs a semantic validator for the named config
// type, to run after schema validation on every create and update. Call during
// controller init; it is not safe to call once requests are being served.
// Registering a second validator for the same type replaces the first.
func (self *ConfigManager) RegisterDataValidator(configTypeName string, v ConfigDataValidator) {
	self.dataValidators[configTypeName] = v
}

// validateData runs the validator registered for configTypeName, if any.
func (self *ConfigManager) validateData(configTypeName string, data map[string]interface{}) error {
	if v, found := self.dataValidators[configTypeName]; found {
		return v(data)
	}
	return nil
}

func (self *ConfigManager) NewModelEntity() *Config {
	return &Config{}
}

func (self *ConfigManager) Create(entity *Config, ctx *change.Context) error {
	return DispatchCreate[*Config](self, entity, ctx)
}

func (self *ConfigManager) ApplyCreate(cmd *command.CreateEntityCommand[*Config], ctx boltz.MutateContext) error {
	_, err := self.createEntity(cmd.Entity, ctx)
	return err
}

func (self *ConfigManager) Update(entity *Config, checker fields.UpdatedFields, ctx *change.Context) error {
	return DispatchUpdate[*Config](self, entity, checker, ctx)
}

func (self *ConfigManager) ApplyUpdate(cmd *command.UpdateEntityCommand[*Config], ctx boltz.MutateContext) error {
	var checker boltz.FieldChecker = self
	if cmd.UpdatedFields != nil {
		checker = &AndFieldChecker{first: self, second: cmd.UpdatedFields}
	}
	return self.updateEntity(cmd.Entity, checker, ctx)
}

func (self *ConfigManager) Read(id string) (*Config, error) {
	modelEntity := &Config{}
	if err := self.readEntity(id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (self *ConfigManager) readInTx(tx *bbolt.Tx, id string) (*Config, error) {
	modelEntity := &Config{}
	if err := self.readEntityInTx(tx, id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (self *ConfigManager) IsUpdated(field string) bool {
	return !strings.EqualFold(field, "type")
}

func (self *ConfigManager) Marshall(entity *Config) ([]byte, error) {
	tags, err := edge_cmd_pb.EncodeTags(entity.Tags)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(entity.Data)
	if err != nil {
		return nil, err
	}

	msg := &edge_cmd_pb.Config{
		Id:           entity.Id,
		Name:         entity.Name,
		ConfigTypeId: entity.TypeId,
		Data:         data,
		Tags:         tags,
	}

	return proto.Marshal(msg)
}

func (self *ConfigManager) Unmarshall(bytes []byte) (*Config, error) {
	msg := &edge_cmd_pb.Config{}
	if err := proto.Unmarshal(bytes, msg); err != nil {
		return nil, err
	}

	data := map[string]interface{}{}
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return nil, err
	}

	return &Config{
		BaseEntity: models.BaseEntity{
			Id:   msg.Id,
			Tags: edge_cmd_pb.DecodeTags(msg.Tags),
		},
		Name:   msg.Name,
		TypeId: msg.ConfigTypeId,
		Data:   data,
	}, nil
}
