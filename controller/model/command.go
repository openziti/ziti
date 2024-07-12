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
	"github.com/openziti/ziti/common/pb/cmd_pb"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/command"
	"github.com/openziti/ziti/controller/fields"
	"github.com/openziti/ziti/controller/idgen"
	"github.com/openziti/ziti/controller/ioc"
	"github.com/openziti/ziti/controller/models"
	"google.golang.org/protobuf/proto"
)

func newCommandManager(env Env, registry ioc.Registry) *CommandManager {
	command.GetDefaultDecoders().Clear()
	result := &CommandManager{
		env:      env,
		registry: registry,
		Decoders: command.GetDefaultDecoders(),
	}
	return result
}

type CommandManager struct {
	env      Env
	registry ioc.Registry
	Decoders command.Decoders
}

func (self *CommandManager) registerGenericCommands() {
	self.Decoders.RegisterF(int32(cmd_pb.CommandType_CreateEntityType), self.decodeCreateEntityCommand)
	self.Decoders.RegisterF(int32(cmd_pb.CommandType_UpdateEntityType), self.decodeUpdateEntityCommand)
	self.Decoders.RegisterF(int32(cmd_pb.CommandType_DeleteEntityType), self.decodeDeleteEntityCommand)
}

func (self *CommandManager) decodeCreateEntityCommand(_ int32, data []byte) (command.Command, error) {
	msg := &cmd_pb.CreateEntityCommand{}
	if err := proto.Unmarshal(data, msg); err != nil {
		return nil, err
	}

	decoder, err := ioc.Get[createDecoderF](self.registry, msg.EntityType+CreateDecoder)
	if err != nil {
		return nil, err
	}

	return decoder(msg)
}

func (self *CommandManager) decodeUpdateEntityCommand(_ int32, data []byte) (command.Command, error) {
	msg := &cmd_pb.UpdateEntityCommand{}
	if err := proto.Unmarshal(data, msg); err != nil {
		return nil, err
	}

	decoder, err := ioc.Get[updateDecoderF](self.registry, msg.EntityType+UpdateDecoder)
	if err != nil {
		return nil, err
	}

	return decoder(msg)
}

func (self *CommandManager) decodeDeleteEntityCommand(_ int32, data []byte) (command.Command, error) {
	msg := &cmd_pb.DeleteEntityCommand{}
	if err := proto.Unmarshal(data, msg); err != nil {
		return nil, err
	}

	decoder, err := ioc.Get[deleteDecoderF](self.registry, msg.EntityType+DeleteDecoder)
	if err != nil {
		return nil, err
	}

	return decoder(msg)
}

// CommandMsg is a TypedMessage which is also a pointer type.
//
// T is message type. We want to enforce that the TypeMessage implementation is a pointer type
// so we can use new(T) to create instances of it
type CommandMsg[T any] interface {
	cmd_pb.TypedMessage
	*T
}

type creator[T models.Entity] interface {
	command.EntityCreator[T]
	Dispatch(cmd command.Command) error
}

type updater[T models.Entity] interface {
	command.EntityUpdater[T]
	Dispatch(cmd command.Command) error
}

func DispatchCreate[T models.Entity](c creator[T], entity T, ctx *change.Context) error {
	if entity.GetId() == "" {
		id := idgen.NewUUIDString()
		entity.SetId(id)
	}

	cmd := &command.CreateEntityCommand[T]{
		Context: ctx,
		Creator: c,
		Entity:  entity,
	}

	return c.Dispatch(cmd)
}

func DispatchUpdate[T models.Entity](u updater[T], entity T, updatedFields fields.UpdatedFields, ctx *change.Context) error {
	cmd := &command.UpdateEntityCommand[T]{
		Context:       ctx,
		Updater:       u,
		Entity:        entity,
		UpdatedFields: updatedFields,
	}

	return u.Dispatch(cmd)
}
