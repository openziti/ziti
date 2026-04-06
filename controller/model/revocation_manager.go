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
	"errors"
	"fmt"
	"time"

	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/common/pb/cmd_pb"
	"github.com/openziti/ziti/common/pb/edge_cmd_pb"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/command"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/models"
	"google.golang.org/protobuf/proto"
)

func NewRevocationManager(env Env) *RevocationManager {
	manager := &RevocationManager{
		baseEntityManager: newBaseEntityManager[*Revocation, *db.Revocation](env, env.GetStores().Revocation),
	}
	manager.impl = manager

	RegisterManagerDecoder[*Revocation](env, manager)
	RegisterCommand(env, &DeleteRevocationsBatchCommand{}, &edge_cmd_pb.DeleteRevocationsBatchCommand{})
	RegisterCommand(env, &CreateRevocationsBatchCommand{}, &edge_cmd_pb.CreateRevocationsBatchCommand{})

	return manager
}

type RevocationManager struct {
	baseEntityManager[*Revocation, *db.Revocation]
}

func (self *RevocationManager) ApplyUpdate(*command.UpdateEntityCommand[*Revocation], boltz.MutateContext) error {
	return errors.New("unsupported")
}

func (self *RevocationManager) Create(entity *Revocation, ctx *change.Context) error {
	return DispatchCreate[*Revocation](self, entity, ctx)
}

func (self *RevocationManager) ApplyCreate(cmd *command.CreateEntityCommand[*Revocation], ctx boltz.MutateContext) error {
	_, err := self.createEntity(cmd.Entity, ctx)
	return err
}

func (self *RevocationManager) newModelEntity() *Revocation {
	return &Revocation{}
}

const revocationDeleteBatchSize = 500

// DeleteExpired deletes all revocations whose ExpiresAt is in the past, working
// in batches of revocationDeleteBatchSize until no expired entries remain.
// Returns the total number of entries deleted.
func (self *RevocationManager) DeleteExpired(ctx *change.Context) (int, error) {
	query := fmt.Sprintf(`expiresAt < datetime(%s) limit %d`, time.Now().UTC().Format(time.RFC3339), revocationDeleteBatchSize)
	total := 0
	for {
		result, err := self.BaseList(query)
		if err != nil {
			return total, err
		}

		ids := make([]string, 0, len(result.GetEntities()))
		for _, entity := range result.GetEntities() {
			ids = append(ids, entity.GetId())
		}

		if len(ids) == 0 {
			break
		}

		if err = self.DeleteBatch(ids, ctx); err != nil {
			return total, err
		}
		total += len(ids)

		if len(ids) < revocationDeleteBatchSize {
			break
		}
	}
	return total, nil
}

func (self *RevocationManager) Read(id string) (*Revocation, error) {
	modelEntity := &Revocation{}
	if err := self.readEntity(id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (self *RevocationManager) Marshall(entity *Revocation) ([]byte, error) {
	tags, err := edge_cmd_pb.EncodeTags(entity.Tags)
	if err != nil {
		return nil, err
	}

	msg := &edge_cmd_pb.Revocation{
		Id:        entity.Id,
		ExpiresAt: timePtrToPb(&entity.ExpiresAt),
		Tags:      tags,
	}

	return proto.Marshal(msg)
}

func (self *RevocationManager) Unmarshall(bytes []byte) (*Revocation, error) {
	msg := &edge_cmd_pb.Revocation{}
	if err := proto.Unmarshal(bytes, msg); err != nil {
		return nil, err
	}

	if msg.ExpiresAt == nil {
		return nil, fmt.Errorf("revocation msg for id '%v' has nil ExpiresAt", msg.Id)
	}

	return &Revocation{
		BaseEntity: models.BaseEntity{
			Id:   msg.Id,
			Tags: edge_cmd_pb.DecodeTags(msg.Tags),
		},
		ExpiresAt: *pbTimeToTimePtr(msg.ExpiresAt),
	}, nil
}

// DeleteBatch dispatches a batched delete of revocation IDs through raft as a
// single log entry and a single DB transaction.
func (self *RevocationManager) DeleteBatch(ids []string, ctx *change.Context) error {
	if len(ids) == 0 {
		return nil
	}
	cmd := &DeleteRevocationsBatchCommand{
		Context: ctx,
		Manager: self,
		Ids:     ids,
	}
	return self.Dispatch(cmd)
}

// ApplyDeleteBatch removes revocations by ID in a single DB transaction.
func (self *RevocationManager) ApplyDeleteBatch(cmd *DeleteRevocationsBatchCommand, ctx boltz.MutateContext) error {
	var errorList []error
	err := self.GetDb().Update(ctx, func(ctx boltz.MutateContext) error {
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
	return errors.Join(errorList...)
}

// DeleteRevocationsBatchCommand deletes a batch of revocations through raft
// in a single log entry and a single DB transaction.
type DeleteRevocationsBatchCommand struct {
	Context *change.Context
	Manager *RevocationManager
	Ids     []string
}

func (self *DeleteRevocationsBatchCommand) Apply(ctx boltz.MutateContext) error {
	return self.Manager.ApplyDeleteBatch(self, ctx)
}

func (self *DeleteRevocationsBatchCommand) Encode() ([]byte, error) {
	return cmd_pb.EncodeProtobuf(&edge_cmd_pb.DeleteRevocationsBatchCommand{
		EntityIds: self.Ids,
		Ctx:       ContextToProtobuf(self.Context),
	})
}

func (self *DeleteRevocationsBatchCommand) Decode(env Env, msg *edge_cmd_pb.DeleteRevocationsBatchCommand) error {
	self.Manager = env.GetManagers().Revocation
	self.Ids = msg.EntityIds
	self.Context = ProtobufToContext(msg.Ctx)
	return nil
}

func (self *DeleteRevocationsBatchCommand) GetChangeContext() *change.Context {
	return self.Context
}

// CreateBatch dispatches a batched create of revocations through raft as a
// single log entry and a single DB transaction.
func (self *RevocationManager) CreateBatch(revocations []*Revocation, ctx *change.Context) error {
	if len(revocations) == 0 {
		return nil
	}
	cmd := &CreateRevocationsBatchCommand{
		Context:     ctx,
		Manager:     self,
		Revocations: revocations,
	}
	return self.Dispatch(cmd)
}

// ApplyCreateBatch persists a batch of revocations in a single DB transaction.
func (self *RevocationManager) ApplyCreateBatch(cmd *CreateRevocationsBatchCommand, ctx boltz.MutateContext) error {
	return self.GetDb().Update(ctx, func(ctx boltz.MutateContext) error {
		for _, rev := range cmd.Revocations {
			if _, err := self.createEntityInTx(ctx, rev); err != nil {
				return err
			}
		}
		return nil
	})
}

// CreateRevocationsBatchCommand creates a batch of revocations through raft
// in a single log entry and a single DB transaction.
type CreateRevocationsBatchCommand struct {
	Context     *change.Context
	Manager     *RevocationManager
	Revocations []*Revocation
}

func (self *CreateRevocationsBatchCommand) Apply(ctx boltz.MutateContext) error {
	return self.Manager.ApplyCreateBatch(self, ctx)
}

func (self *CreateRevocationsBatchCommand) Encode() ([]byte, error) {
	var pbRevocations []*edge_cmd_pb.Revocation
	for _, rev := range self.Revocations {
		tags, err := edge_cmd_pb.EncodeTags(rev.Tags)
		if err != nil {
			return nil, err
		}
		pbRevocations = append(pbRevocations, &edge_cmd_pb.Revocation{
			Id:        rev.Id,
			ExpiresAt: timePtrToPb(&rev.ExpiresAt),
			Tags:      tags,
		})
	}
	return cmd_pb.EncodeProtobuf(&edge_cmd_pb.CreateRevocationsBatchCommand{
		Revocations: pbRevocations,
		Ctx:         ContextToProtobuf(self.Context),
	})
}

func (self *CreateRevocationsBatchCommand) Decode(env Env, msg *edge_cmd_pb.CreateRevocationsBatchCommand) error {
	self.Manager = env.GetManagers().Revocation
	self.Context = ProtobufToContext(msg.Ctx)
	for _, pbRev := range msg.Revocations {
		if pbRev.ExpiresAt == nil {
			return fmt.Errorf("revocation batch entry for id '%v' has nil ExpiresAt", pbRev.Id)
		}
		rev := &Revocation{
			BaseEntity: models.BaseEntity{
				Id:   pbRev.Id,
				Tags: edge_cmd_pb.DecodeTags(pbRev.Tags),
			},
			ExpiresAt: *pbTimeToTimePtr(pbRev.ExpiresAt),
		}
		self.Revocations = append(self.Revocations, rev)
	}
	return nil
}

func (self *CreateRevocationsBatchCommand) GetChangeContext() *change.Context {
	return self.Context
}
