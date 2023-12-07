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
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/common/pb/edge_cmd_pb"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/command"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/fields"
	"github.com/openziti/ziti/controller/models"
	"github.com/openziti/ziti/controller/network"
	"google.golang.org/protobuf/proto"
)

func NewServicePolicyManager(env Env) *ServicePolicyManager {
	manager := &ServicePolicyManager{
		baseEntityManager: newBaseEntityManager[*ServicePolicy, *db.ServicePolicy](env, env.GetStores().ServicePolicy),
	}
	manager.impl = manager

	network.RegisterManagerDecoder[*ServicePolicy](env.GetHostController().GetNetwork().Managers, manager)

	return manager
}

type ServicePolicyManager struct {
	baseEntityManager[*ServicePolicy, *db.ServicePolicy]
}

func (self *ServicePolicyManager) newModelEntity() *ServicePolicy {
	return &ServicePolicy{}
}

func (self *ServicePolicyManager) Create(entity *ServicePolicy, ctx *change.Context) error {
	return network.DispatchCreate[*ServicePolicy](self, entity, ctx)
}

func (self *ServicePolicyManager) ApplyCreate(cmd *command.CreateEntityCommand[*ServicePolicy], ctx boltz.MutateContext) error {
	_, err := self.createEntity(cmd.Entity, ctx)
	return err
}

func (self *ServicePolicyManager) Update(entity *ServicePolicy, checker fields.UpdatedFields, ctx *change.Context) error {
	return network.DispatchUpdate[*ServicePolicy](self, entity, checker, ctx)
}

func (self *ServicePolicyManager) ApplyUpdate(cmd *command.UpdateEntityCommand[*ServicePolicy], ctx boltz.MutateContext) error {
	return self.updateEntity(cmd.Entity, cmd.UpdatedFields, ctx)
}

func (self *ServicePolicyManager) Marshall(entity *ServicePolicy) ([]byte, error) {
	tags, err := edge_cmd_pb.EncodeTags(entity.Tags)
	if err != nil {
		return nil, err
	}

	msg := &edge_cmd_pb.ServicePolicy{
		Id:                entity.Id,
		Name:              entity.Name,
		Tags:              tags,
		Semantic:          entity.Semantic,
		IdentityRoles:     entity.IdentityRoles,
		ServiceRoles:      entity.ServiceRoles,
		PostureCheckRoles: entity.PostureCheckRoles,
		PolicyType:        entity.PolicyType,
	}

	return proto.Marshal(msg)
}

func (self *ServicePolicyManager) Unmarshall(bytes []byte) (*ServicePolicy, error) {
	msg := &edge_cmd_pb.ServicePolicy{}
	if err := proto.Unmarshal(bytes, msg); err != nil {
		return nil, err
	}

	return &ServicePolicy{
		BaseEntity: models.BaseEntity{
			Id:   msg.Id,
			Tags: edge_cmd_pb.DecodeTags(msg.Tags),
		},
		Name:              msg.Name,
		Semantic:          msg.Semantic,
		IdentityRoles:     msg.IdentityRoles,
		ServiceRoles:      msg.ServiceRoles,
		PostureCheckRoles: msg.PostureCheckRoles,
		PolicyType:        msg.PolicyType,
	}, nil
}
