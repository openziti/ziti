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

func NewEdgeRouterPolicyManager(env Env) *EdgeRouterPolicyManager {
	manager := &EdgeRouterPolicyManager{
		baseEntityManager: newBaseEntityManager(env, env.GetStores().EdgeRouterPolicy),
	}
	manager.impl = manager

	network.RegisterManagerDecoder[*EdgeRouterPolicy](env.GetHostController().GetNetwork().Managers, manager)

	return manager
}

type EdgeRouterPolicyManager struct {
	baseEntityManager
}

func (self *EdgeRouterPolicyManager) newModelEntity() edgeEntity {
	return &EdgeRouterPolicy{}
}

func (self *EdgeRouterPolicyManager) Create(entity *EdgeRouterPolicy) error {
	return network.DispatchCreate[*EdgeRouterPolicy](self, entity)
}

func (self *EdgeRouterPolicyManager) ApplyCreate(cmd *command.CreateEntityCommand[*EdgeRouterPolicy]) error {
	_, err := self.createEntity(cmd.Entity)
	return err
}

func (self *EdgeRouterPolicyManager) Update(entity *EdgeRouterPolicy, checker fields.UpdatedFields) error {
	return network.DispatchUpdate[*EdgeRouterPolicy](self, entity, checker)
}

func (self *EdgeRouterPolicyManager) ApplyUpdate(cmd *command.UpdateEntityCommand[*EdgeRouterPolicy]) error {
	return self.updateEntity(cmd.Entity, cmd.UpdatedFields)
}

func (self *EdgeRouterPolicyManager) Read(id string) (*EdgeRouterPolicy, error) {
	modelEntity := &EdgeRouterPolicy{}
	if err := self.readEntity(id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (self *EdgeRouterPolicyManager) readInTx(tx *bbolt.Tx, id string) (*EdgeRouterPolicy, error) {
	modelEntity := &EdgeRouterPolicy{}
	if err := self.readEntityInTx(tx, id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (self *EdgeRouterPolicyManager) Marshall(entity *EdgeRouterPolicy) ([]byte, error) {
	tags, err := edge_cmd_pb.EncodeTags(entity.Tags)
	if err != nil {
		return nil, err
	}

	msg := &edge_cmd_pb.EdgeRouterPolicy{
		Id:              entity.Id,
		Name:            entity.Name,
		Tags:            tags,
		Semantic:        entity.Semantic,
		EdgeRouterRoles: entity.EdgeRouterRoles,
		IdentityRoles:   entity.IdentityRoles,
	}

	return proto.Marshal(msg)
}

func (self *EdgeRouterPolicyManager) Unmarshall(bytes []byte) (*EdgeRouterPolicy, error) {
	msg := &edge_cmd_pb.EdgeRouterPolicy{}
	if err := proto.Unmarshal(bytes, msg); err != nil {
		return nil, err
	}

	return &EdgeRouterPolicy{
		BaseEntity: models.BaseEntity{
			Id:   msg.Id,
			Tags: edge_cmd_pb.DecodeTags(msg.Tags),
		},
		Name:            msg.Name,
		Semantic:        msg.Semantic,
		IdentityRoles:   msg.IdentityRoles,
		EdgeRouterRoles: msg.EdgeRouterRoles,
	}, nil
}
