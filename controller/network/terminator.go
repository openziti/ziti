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

package network

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/change"
	"github.com/openziti/fabric/controller/command"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/controller/fields"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/fabric/pb/cmd_pb"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"google.golang.org/protobuf/proto"
	"reflect"
	"strings"
)

type Terminator struct {
	models.BaseEntity
	Service        string
	Router         string
	Binding        string
	Address        string
	InstanceId     string
	InstanceSecret []byte
	Cost           uint16
	Precedence     xt.Precedence
	PeerData       map[uint32][]byte
	HostId         string
}

func (entity *Terminator) GetServiceId() string {
	return entity.Service
}

func (entity *Terminator) GetRouterId() string {
	return entity.Router
}

func (entity *Terminator) GetBinding() string {
	return entity.Binding
}

func (entity *Terminator) GetAddress() string {
	return entity.Address
}

func (entity *Terminator) GetInstanceId() string {
	return entity.InstanceId
}

func (entity *Terminator) GetInstanceSecret() []byte {
	return entity.InstanceSecret
}

func (entity *Terminator) GetCost() uint16 {
	return entity.Cost
}

func (entity *Terminator) GetPrecedence() xt.Precedence {
	return entity.Precedence
}

func (entity *Terminator) GetPeerData() xt.PeerData {
	return entity.PeerData
}

func (entity *Terminator) GetHostId() string {
	return entity.HostId
}

func (entity *Terminator) toBolt() *db.Terminator {
	precedence := xt.Precedences.Default.String()
	if entity.Precedence != nil {
		precedence = entity.Precedence.String()
	}
	return &db.Terminator{
		BaseExtEntity:  *entity.ToBoltBaseExtEntity(),
		Service:        entity.Service,
		Router:         entity.Router,
		Binding:        entity.Binding,
		Address:        entity.Address,
		InstanceId:     entity.InstanceId,
		InstanceSecret: entity.InstanceSecret,
		Cost:           entity.Cost,
		Precedence:     precedence,
		PeerData:       entity.PeerData,
		HostId:         entity.HostId,
	}
}

func newTerminatorManager(managers *Managers) *TerminatorManager {
	result := &TerminatorManager{
		baseEntityManager: newBaseEntityManager[*Terminator, *db.Terminator](managers, managers.stores.Terminator, func() *Terminator {
			return &Terminator{}
		}),
		store: managers.stores.Terminator,
	}
	result.populateEntity = result.populateTerminator

	managers.stores.Terminator.AddEntityIdListener(xt.GlobalCosts().ClearCost, boltz.EntityDeleted)

	return result
}

type TerminatorManager struct {
	baseEntityManager[*Terminator, *db.Terminator]
	store db.TerminatorStore
}

func (self *TerminatorManager) Create(entity *Terminator, ctx *change.Context) error {
	return DispatchCreate[*Terminator](self, entity, ctx)
}

func (self *TerminatorManager) ApplyCreate(cmd *command.CreateEntityCommand[*Terminator], ctx boltz.MutateContext) error {
	return self.db.Update(ctx, func(ctx boltz.MutateContext) error {
		if cmd.Entity.IsSystemEntity() {
			ctx = ctx.GetSystemContext()
		}
		self.checkBinding(cmd.Entity)
		boltTerminator := cmd.Entity.toBolt()
		err := self.GetStore().Create(ctx, boltTerminator)
		if err != nil {
			return err
		}
		if cmd.PostCreateHook != nil {
			return cmd.PostCreateHook(ctx, cmd.Entity)
		}
		return nil
	})
}

func (self *TerminatorManager) DeleteBatch(ids []string, ctx *change.Context) error {
	cmd := &DeleteTerminatorsBatchCommand{
		Context: ctx,
		Manager: self,
		Ids:     ids,
	}
	return self.Managers.Dispatch(cmd)
}

func (self *TerminatorManager) ApplyDeleteBatch(cmd *DeleteTerminatorsBatchCommand, ctx boltz.MutateContext) error {
	var errorList errorz.MultipleErrors
	err := self.db.Update(ctx, func(ctx boltz.MutateContext) error {
		for _, id := range cmd.Ids {
			if self.Store.IsEntityPresent(ctx.Tx(), id) {
				if err := self.Store.DeleteById(ctx, id); err != nil {
					errorList = append(errorList, err)
				}
			}
		}
		return nil
	})
	if err != nil {
		errorList = append(errorList, err)
	}
	return errorList.ToError()
}

func (self *TerminatorManager) checkBinding(terminator *Terminator) {
	if terminator.Binding == "" {
		if strings.HasPrefix(terminator.Address, "udp:") {
			terminator.Binding = "udp"
		} else {
			terminator.Binding = "transport"
		}
	}
}

func (self *TerminatorManager) handlePrecedenceChange(terminatorId string, precedence xt.Precedence) {
	terminator, err := self.Read(terminatorId)
	if err != nil {
		pfxlog.Logger().Errorf("unable to update precedence for terminator %v to %v (%v)",
			terminatorId, precedence, err)
		return
	}

	terminator.Precedence = precedence
	checker := fields.UpdatedFieldsMap{
		db.FieldTerminatorPrecedence: struct{}{},
	}

	if err = self.Update(terminator, checker, change.New().SetSourceType(change.SourceTypeXt).SetChangeAuthorType(change.AuthorTypeController)); err != nil {
		pfxlog.Logger().Errorf("unable to update precedence for terminator %v to %v (%v)", terminatorId, precedence, err)
	}
}

func (self *TerminatorManager) Update(entity *Terminator, updatedFields fields.UpdatedFields, ctx *change.Context) error {
	return DispatchUpdate[*Terminator](self, entity, updatedFields, ctx)
}

func (self *TerminatorManager) ApplyUpdate(cmd *command.UpdateEntityCommand[*Terminator], ctx boltz.MutateContext) error {
	terminator := cmd.Entity
	return self.db.Update(ctx, func(ctx boltz.MutateContext) error {
		if cmd.Entity.IsSystemEntity() {
			ctx = ctx.GetSystemContext()
		}

		self.checkBinding(terminator)
		return self.GetStore().Update(ctx, terminator.toBolt(), cmd.UpdatedFields)
	})
}

func (self *TerminatorManager) Read(id string) (entity *Terminator, err error) {
	err = self.db.View(func(tx *bbolt.Tx) error {
		entity, err = self.readInTx(tx, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return entity, err
}

func (self *TerminatorManager) readInTx(tx *bbolt.Tx, id string) (*Terminator, error) {
	entity := &Terminator{}
	err := self.readEntityInTx(tx, id, entity)
	if err != nil {
		return nil, err
	}
	return entity, nil
}

func (self *TerminatorManager) Query(query string) (*TerminatorListResult, error) {
	result := &TerminatorListResult{controller: self}
	if err := self.ListWithHandler(query, result.collect); err != nil {
		return nil, err
	}
	return result, nil
}

func (self *TerminatorManager) populateTerminator(entity *Terminator, _ *bbolt.Tx, boltEntity boltz.Entity) error {
	boltTerminator, ok := boltEntity.(*db.Terminator)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model terminator", reflect.TypeOf(boltEntity))
	}
	entity.Service = boltTerminator.Service
	entity.Router = boltTerminator.Router
	entity.Binding = boltTerminator.Binding
	entity.Address = boltTerminator.Address
	entity.InstanceId = boltTerminator.InstanceId
	entity.InstanceSecret = boltTerminator.InstanceSecret
	entity.PeerData = boltTerminator.PeerData
	entity.Cost = boltTerminator.Cost
	entity.Precedence = xt.GetPrecedenceForName(boltTerminator.Precedence)
	entity.HostId = boltTerminator.HostId
	entity.FillCommon(boltTerminator)
	return nil
}

func (self *TerminatorManager) Marshall(entity *Terminator) ([]byte, error) {
	tags, err := cmd_pb.EncodeTags(entity.Tags)
	if err != nil {
		return nil, err
	}

	var precedence uint32
	if entity.Precedence != nil {
		if entity.Precedence.IsFailed() {
			precedence = 1
		} else if entity.Precedence.IsRequired() {
			precedence = 2
		}
	}

	msg := &cmd_pb.Terminator{
		Id:             entity.Id,
		ServiceId:      entity.GetServiceId(),
		RouterId:       entity.GetRouterId(),
		Binding:        entity.Binding,
		Address:        entity.Address,
		InstanceId:     entity.InstanceId,
		InstanceSecret: entity.InstanceSecret,
		Cost:           uint32(entity.Cost),
		Precedence:     precedence,
		PeerData:       entity.PeerData,
		Tags:           tags,
		HostId:         entity.HostId,
		IsSystem:       entity.IsSystem,
	}

	return proto.Marshal(msg)
}

func (self *TerminatorManager) Unmarshall(bytes []byte) (*Terminator, error) {
	msg := &cmd_pb.Terminator{}
	if err := proto.Unmarshal(bytes, msg); err != nil {
		return nil, err
	}

	precedence := xt.Precedences.Default
	if msg.Precedence == 1 {
		precedence = xt.Precedences.Failed
	} else if msg.Precedence == 2 {
		precedence = xt.Precedences.Required
	}

	result := &Terminator{
		BaseEntity: models.BaseEntity{
			Id:       msg.Id,
			Tags:     cmd_pb.DecodeTags(msg.Tags),
			IsSystem: msg.IsSystem,
		},
		Service:        msg.ServiceId,
		Router:         msg.RouterId,
		Binding:        msg.Binding,
		Address:        msg.Address,
		InstanceId:     msg.InstanceId,
		InstanceSecret: msg.InstanceSecret,
		Cost:           uint16(msg.Cost),
		Precedence:     precedence,
		PeerData:       msg.PeerData,
		HostId:         msg.HostId,
	}

	return result, nil
}

type TerminatorListResult struct {
	controller *TerminatorManager
	Entities   []*Terminator
	models.QueryMetaData
}

func (result *TerminatorListResult) collect(tx *bbolt.Tx, ids []string, qmd *models.QueryMetaData) error {
	result.QueryMetaData = *qmd
	for _, id := range ids {
		terminator, err := result.controller.readInTx(tx, id)
		if err != nil {
			return err
		}
		result.Entities = append(result.Entities, terminator)
	}
	return nil
}

type RoutingTerminator struct {
	RouteCost uint32
	*Terminator
}

func (r *RoutingTerminator) GetRouteCost() uint32 {
	return r.RouteCost
}

type DeleteTerminatorsBatchCommand struct {
	Context *change.Context
	Manager *TerminatorManager
	Ids     []string
}

func (self *DeleteTerminatorsBatchCommand) Apply(ctx boltz.MutateContext) error {
	return self.Manager.ApplyDeleteBatch(self, ctx)
}

func (self *DeleteTerminatorsBatchCommand) Encode() ([]byte, error) {
	return cmd_pb.EncodeProtobuf(&cmd_pb.DeleteTerminatorsBatchCommand{
		EntityIds: self.Ids,
	})
}

func (self *DeleteTerminatorsBatchCommand) Decode(n *Network, msg *cmd_pb.DeleteTerminatorsBatchCommand) error {
	self.Manager = n.Terminators
	self.Ids = msg.EntityIds
	return nil
}

func (self *DeleteTerminatorsBatchCommand) GetChangeContext() *change.Context {
	return self.Context
}
