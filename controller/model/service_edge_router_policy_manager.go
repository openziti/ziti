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
	"github.com/openziti/edge/pb/edge_cmd_pb"
	"github.com/openziti/fabric/controller/command"
	"github.com/openziti/fabric/controller/fields"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/fabric/controller/network"
	"go.etcd.io/bbolt"
	"google.golang.org/protobuf/proto"
)

func NewServiceEdgeRouterPolicyManager(env Env) *ServiceEdgeRouterPolicyManager {
	manager := &ServiceEdgeRouterPolicyManager{
		baseEntityManager: newBaseEntityManager(env, env.GetStores().ServiceEdgeRouterPolicy),
	}
	manager.impl = manager

	network.RegisterManagerDecoder[*ServiceEdgeRouterPolicy](env.GetHostController().GetNetwork().Managers, manager)

	return manager
}

type ServiceEdgeRouterPolicyManager struct {
	baseEntityManager
}

func (self *ServiceEdgeRouterPolicyManager) newModelEntity() edgeEntity {
	return &ServiceEdgeRouterPolicy{}
}

func (self *ServiceEdgeRouterPolicyManager) Create(entity *ServiceEdgeRouterPolicy) error {
	return network.DispatchCreate[*ServiceEdgeRouterPolicy](self, entity)
}

func (self *ServiceEdgeRouterPolicyManager) ApplyCreate(cmd *command.CreateEntityCommand[*ServiceEdgeRouterPolicy]) error {
	_, err := self.createEntity(cmd.Entity)
	return err
}

func (self *ServiceEdgeRouterPolicyManager) Update(entity *ServiceEdgeRouterPolicy, checker fields.UpdatedFields) error {
	return network.DispatchUpdate[*ServiceEdgeRouterPolicy](self, entity, checker)
}

func (self *ServiceEdgeRouterPolicyManager) ApplyUpdate(cmd *command.UpdateEntityCommand[*ServiceEdgeRouterPolicy]) error {
	return self.updateEntity(cmd.Entity, cmd.UpdatedFields)
}

func (self *ServiceEdgeRouterPolicyManager) Read(id string) (*ServiceEdgeRouterPolicy, error) {
	modelEntity := &ServiceEdgeRouterPolicy{}
	if err := self.readEntity(id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (self *ServiceEdgeRouterPolicyManager) readInTx(tx *bbolt.Tx, id string) (*ServiceEdgeRouterPolicy, error) {
	modelEntity := &ServiceEdgeRouterPolicy{}
	if err := self.readEntityInTx(tx, id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (self *ServiceEdgeRouterPolicyManager) Marshall(entity *ServiceEdgeRouterPolicy) ([]byte, error) {
	tags, err := edge_cmd_pb.EncodeTags(entity.Tags)
	if err != nil {
		return nil, err
	}

	msg := &edge_cmd_pb.ServiceEdgeRouterPolicy{
		Id:              entity.Id,
		Name:            entity.Name,
		Tags:            tags,
		Semantic:        entity.Semantic,
		EdgeRouterRoles: entity.EdgeRouterRoles,
		ServiceRoles:    entity.ServiceRoles,
	}

	return proto.Marshal(msg)
}

func (self *ServiceEdgeRouterPolicyManager) Unmarshall(bytes []byte) (*ServiceEdgeRouterPolicy, error) {
	msg := &edge_cmd_pb.ServiceEdgeRouterPolicy{}
	if err := proto.Unmarshal(bytes, msg); err != nil {
		return nil, err
	}

	return &ServiceEdgeRouterPolicy{
		BaseEntity: models.BaseEntity{
			Id:   msg.Id,
			Tags: edge_cmd_pb.DecodeTags(msg.Tags),
		},
		Name:            msg.Name,
		Semantic:        msg.Semantic,
		EdgeRouterRoles: msg.EdgeRouterRoles,
		ServiceRoles:    msg.ServiceRoles,
	}, nil
}
