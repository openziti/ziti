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
	"context"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/channel/v2/protobufs"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/common/pb/cmd_pb"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/common/pb/mgmt_pb"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/command"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/fields"
	"github.com/openziti/ziti/controller/models"
	"github.com/openziti/ziti/controller/xt"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"google.golang.org/protobuf/proto"
	"reflect"
	"strings"
	"time"
)

type Terminator struct {
	models.BaseEntity
	Service         string
	Router          string
	Binding         string
	Address         string
	InstanceId      string
	InstanceSecret  []byte
	Cost            uint16
	Precedence      xt.Precedence
	PeerData        map[uint32][]byte
	HostId          string
	SavedPrecedence xt.Precedence
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

	var savedPrecedence *string
	if entity.SavedPrecedence != nil {
		precedenceStr := entity.SavedPrecedence.String()
		savedPrecedence = &precedenceStr
	}

	return &db.Terminator{
		BaseExtEntity:   *entity.ToBoltBaseExtEntity(),
		Service:         entity.Service,
		Router:          entity.Router,
		Binding:         entity.Binding,
		Address:         entity.Address,
		InstanceId:      entity.InstanceId,
		InstanceSecret:  entity.InstanceSecret,
		Cost:            entity.Cost,
		Precedence:      precedence,
		PeerData:        entity.PeerData,
		HostId:          entity.HostId,
		SavedPrecedence: savedPrecedence,
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

	if boltTerminator.SavedPrecedence != nil {
		entity.SavedPrecedence = xt.GetPrecedenceForName(*boltTerminator.SavedPrecedence)
	}

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

	var savedPrecedence uint32
	if entity.SavedPrecedence != nil {
		if entity.SavedPrecedence.IsFailed() {
			savedPrecedence = 1
		} else if entity.SavedPrecedence.IsRequired() {
			savedPrecedence = 2
		} else if entity.SavedPrecedence.IsDefault() {
			savedPrecedence = 3
		}
	}

	msg := &cmd_pb.Terminator{
		Id:              entity.Id,
		ServiceId:       entity.GetServiceId(),
		RouterId:        entity.GetRouterId(),
		Binding:         entity.Binding,
		Address:         entity.Address,
		InstanceId:      entity.InstanceId,
		InstanceSecret:  entity.InstanceSecret,
		Cost:            uint32(entity.Cost),
		Precedence:      precedence,
		PeerData:        entity.PeerData,
		Tags:            tags,
		HostId:          entity.HostId,
		IsSystem:        entity.IsSystem,
		SavedPrecedence: savedPrecedence,
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

	var savedPrecedence xt.Precedence
	if msg.SavedPrecedence == 1 {
		savedPrecedence = xt.Precedences.Failed
	} else if msg.SavedPrecedence == 2 {
		savedPrecedence = xt.Precedences.Required
	} else if msg.SavedPrecedence == 3 {
		savedPrecedence = xt.Precedences.Default
	}

	result := &Terminator{
		BaseEntity: models.BaseEntity{
			Id:       msg.Id,
			Tags:     cmd_pb.DecodeTags(msg.Tags),
			IsSystem: msg.IsSystem,
		},
		Service:         msg.ServiceId,
		Router:          msg.RouterId,
		Binding:         msg.Binding,
		Address:         msg.Address,
		InstanceId:      msg.InstanceId,
		InstanceSecret:  msg.InstanceSecret,
		Cost:            uint16(msg.Cost),
		Precedence:      precedence,
		PeerData:        msg.PeerData,
		HostId:          msg.HostId,
		SavedPrecedence: savedPrecedence,
	}

	return result, nil
}

type TerminatorValidationCallback func(detail *mgmt_pb.TerminatorDetail)

func (self *TerminatorManager) ValidateTerminators(filter string, fixInvalid bool, cb TerminatorValidationCallback) (uint64, error) {
	if filter == "" {
		filter = "true limit none"
	}
	result, err := self.BaseList(filter)
	if err != nil {
		return 0, err
	}

	go func() {
		batches := map[string][]*Terminator{}

		for _, terminator := range result.Entities {
			routerId := terminator.Router
			batch := append(batches[routerId], terminator)
			batches[routerId] = batch
			if len(batch) == 50 {
				self.validateTerminatorBatch(fixInvalid, routerId, batch, cb)
				delete(batches, routerId)
			}
		}

		for routerId, batch := range batches {
			self.validateTerminatorBatch(fixInvalid, routerId, batch, cb)
		}
	}()

	return uint64(len(result.Entities)), nil
}

func (self *TerminatorManager) validateTerminatorBatch(fixInvalid bool, routerId string, batch []*Terminator, cb TerminatorValidationCallback) {
	router := self.Managers.Routers.getConnected(routerId)
	if router == nil {
		self.reportError(router, batch, cb, "router off-line")
		return
	}

	request := &ctrl_pb.ValidateTerminatorsV2Request{
		FixInvalid: fixInvalid,
	}
	for _, terminator := range batch {
		request.Terminators = append(request.Terminators, &ctrl_pb.Terminator{
			Id:      terminator.Id,
			Binding: terminator.Binding,
			Address: terminator.Address,
		})
	}

	b, err := proto.Marshal(request)
	if err != nil {
		self.reportError(router, batch, cb, fmt.Sprintf("failed to marshal %s: %s", reflect.TypeOf(request), err.Error()))
		return
	}

	msg := channel.NewMessage(int32(ctrl_pb.ContentType_ValidateTerminatorsV2RequestType), b)
	envelope := &ValidateTerminatorRequestSendable{
		Message:     msg,
		fixInvalid:  fixInvalid,
		cb:          cb,
		mgr:         self,
		router:      router,
		terminators: batch,
	}
	envelope.ctx, envelope.cancelF = context.WithTimeout(context.Background(), time.Minute)

	if err = router.Control.Send(envelope); err != nil {
		self.reportError(router, batch, cb, fmt.Sprintf("failed to send %s: %s", reflect.TypeOf(request), err.Error()))
		return
	}
}

func (self *TerminatorManager) reportError(router *Router, batch []*Terminator, cb TerminatorValidationCallback, err string) {
	for _, terminator := range batch {
		detail := self.newTerminatorDetail(router, terminator)
		detail.State = mgmt_pb.TerminatorState_Unknown
		detail.Detail = err
		cb(detail)
	}
}

func (self *TerminatorManager) newTerminatorDetail(router *Router, terminator *Terminator) *mgmt_pb.TerminatorDetail {
	detail := &mgmt_pb.TerminatorDetail{
		TerminatorId: terminator.Id,
		ServiceId:    terminator.Service,
		ServiceName:  "unable to retrieve",
		RouterId:     terminator.Router,
		RouterName:   "unable to retrieve",
		Binding:      terminator.Binding,
		Address:      terminator.Address,
		HostId:       terminator.HostId,
		CreateDate:   terminator.CreatedAt.Format(time.RFC3339),
	}

	service, _ := self.Services.Read(terminator.Service)
	if service != nil {
		detail.ServiceName = service.Name
	}

	if router == nil {
		router, _ = self.Routers.Read(terminator.Router)
	}

	if router != nil {
		detail.RouterName = router.Name
	}

	return detail
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

type ValidateTerminatorRequestSendable struct {
	channel.BaseSendListener
	*channel.Message
	fixInvalid  bool
	mgr         *TerminatorManager
	router      *Router
	terminators []*Terminator
	cb          TerminatorValidationCallback
	ctx         context.Context
	cancelF     func()
}

func (self *ValidateTerminatorRequestSendable) AcceptReply(message *channel.Message) {
	self.cancelF()

	response := &ctrl_pb.ValidateTerminatorsV2Response{}
	if err := protobufs.TypedResponse(response).Unmarshall(message, nil); err != nil {
		pfxlog.Logger().WithError(err).Error("unable to unmarshall validate terminators v2 response")
		return
	}

	var invalidIds []string

	for _, terminator := range self.terminators {
		if status := response.States[terminator.Id]; status != nil && !status.Valid {
			invalidIds = append(invalidIds, terminator.Id)
		}
	}

	fixed := false

	if self.fixInvalid && len(invalidIds) > 0 {
		// todo: figure out how to inject change context from outside of websocket context
		changeCtx := change.New().SetSourceType(change.SourceTypeWebSocket).SetChangeAuthorId(change.AuthorTypeUnattributed)
		err := self.mgr.DeleteBatch(invalidIds, changeCtx)
		if err != nil {
			pfxlog.Logger().WithError(err).Error("unable to batch delete invalid terminators")
		} else {
			fixed = true
		}
	}

	for _, terminator := range self.terminators {
		detail := self.mgr.newTerminatorDetail(self.router, terminator)
		if status := response.States[terminator.Id]; status != nil {
			if status.Valid {
				detail.State = mgmt_pb.TerminatorState_Valid
			} else if status.Reason == ctrl_pb.TerminatorInvalidReason_UnknownBinding {
				detail.State = mgmt_pb.TerminatorState_InvalidUnknownBinding
			} else if status.Reason == ctrl_pb.TerminatorInvalidReason_UnknownTerminator {
				detail.State = mgmt_pb.TerminatorState_InvalidUnknownTerminator
			} else if status.Reason == ctrl_pb.TerminatorInvalidReason_BadState {
				detail.State = mgmt_pb.TerminatorState_InvalidBadState
			} else {
				detail.State = mgmt_pb.TerminatorState_Unknown
			}

			if !status.Valid {
				detail.Fixed = fixed
			}
			detail.Detail = status.Detail
		} else {
			detail.State = mgmt_pb.TerminatorState_Unknown
		}
		self.cb(detail)
	}
}

func (self *ValidateTerminatorRequestSendable) Context() context.Context {
	return self.ctx
}

func (self *ValidateTerminatorRequestSendable) SendListener() channel.SendListener {
	return self
}

func (self *ValidateTerminatorRequestSendable) ReplyReceiver() channel.ReplyReceiver {
	return self
}
