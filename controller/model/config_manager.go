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
	"github.com/openziti/edge/pb/edge_cmd_pb"
	"github.com/openziti/fabric/controller/command"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/storage/boltz"
	"go.etcd.io/bbolt"
	"google.golang.org/protobuf/proto"
	"strings"
)

func NewConfigManager(env Env) *ConfigManager {
	handler := &ConfigManager{
		baseEntityManager: newBaseEntityManager(env, env.GetStores().Config),
	}
	handler.impl = handler

	network.RegisterManagerDecoder[*Config](env.GetHostController().GetNetwork().Managers, handler)

	return handler
}

type ConfigManager struct {
	baseEntityManager
}

func (self *ConfigManager) ApplyUpdate(cmd *command.UpdateEntityCommand[*Config]) error {
	var checker boltz.FieldChecker = cmd.UpdatedFields
	if checker != nil {
		checker = &AndFieldChecker{first: self, second: checker}
	}
	return self.updateEntity(cmd.Entity, checker)
}

func (self *ConfigManager) newModelEntity() boltEntitySink {
	return &Config{}
}

func (self *ConfigManager) Create(config *Config) error {
	return network.DispatchCreate[*Config](self, config)
}

func (self *ConfigManager) ApplyCreate(cmd *command.CreateEntityCommand[*Config]) error {
	_, err := self.createEntity(cmd.Entity)
	return err
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

func (self *ConfigManager) Update(config *Config, checker boltz.UpdatedFields) error {
	return network.DispatchUpdate[*Config](self, config, checker)
}

func (self *ConfigManager) Marshall(entity *Config) ([]byte, error) {
	tags, err := edge_cmd_pb.EncodeTags(entity.Tags)
	if err != nil {
		return nil, err
	}

	data, err := edge_cmd_pb.EncodeJson(entity.Data)
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

	return &Config{
		BaseEntity: models.BaseEntity{
			Id:   msg.Id,
			Tags: edge_cmd_pb.DecodeTags(msg.Tags),
		},
		Name:   msg.Name,
		TypeId: msg.ConfigTypeId,
		Data:   edge_cmd_pb.DecodeJson(msg.Data),
	}, nil
}

type ConfigListResult struct {
	Configs []*Config
	models.QueryMetaData
}
