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
	"github.com/openziti/fabric/controller/command"
	"github.com/openziti/fabric/controller/ioc"
	"github.com/openziti/fabric/common/pb/cmd_pb"
	"google.golang.org/protobuf/proto"
)

func newCommandManager(managers *Managers) *CommandManager {
	command.GetDefaultDecoders().Clear()
	result := &CommandManager{
		Managers: managers,
		Decoders: command.GetDefaultDecoders(),
	}
	return result
}

type CommandManager struct {
	*Managers
	Decoders command.Decoders
}

func (self *CommandManager) registerGenericCommands() {
	self.Decoders.RegisterF(int32(cmd_pb.CommandType_CreateEntityType), self.decodeCreateEntityCommand)
	self.Decoders.RegisterF(int32(cmd_pb.CommandType_UpdateEntityType), self.decodeUpdateEntityCommand)
	self.Decoders.RegisterF(int32(cmd_pb.CommandType_DeleteEntityType), self.decodeDeleteEntityCommand)
	self.Decoders.RegisterF(int32(cmd_pb.CommandType_SyncSnapshot), self.decodeSyncSnapshotCommand)
}

func (self *CommandManager) decodeCreateEntityCommand(_ int32, data []byte) (command.Command, error) {
	msg := &cmd_pb.CreateEntityCommand{}
	if err := proto.Unmarshal(data, msg); err != nil {
		return nil, err
	}

	decoder, err := ioc.Get[createDecoderF](self.Registry, msg.EntityType+CreateDecoder)
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

	decoder, err := ioc.Get[updateDecoderF](self.Registry, msg.EntityType+UpdateDecoder)
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

	decoder, err := ioc.Get[deleteDecoderF](self.Registry, msg.EntityType+DeleteDecoder)
	if err != nil {
		return nil, err
	}

	return decoder(msg)
}

func (self *CommandManager) decodeSyncSnapshotCommand(_ int32, data []byte) (command.Command, error) {
	msg := &cmd_pb.SyncSnapshotCommand{}
	if err := proto.Unmarshal(data, msg); err != nil {
		return nil, err
	}

	cmd := &command.SyncSnapshotCommand{
		SnapshotId:   msg.SnapshotId,
		Snapshot:     msg.Snapshot,
		SnapshotSink: self.network.RestoreSnapshot,
	}

	return cmd, nil
}

// CommandMsg is a TypedMessage which is also a pointer type.
//
// T is message type. We want to enforce that the TypeMessage implementation is a pointer type
// so we can use new(T) to create instances of it
type CommandMsg[T any] interface {
	cmd_pb.TypedMessage
	*T
}

// decodableCommand is a Command which knows how to decode itself from the given message type
//
// T is the type of the command. We want to enforce that the command is a pointer type so we can
// use new(T) to create new instances of it
// M is the message type that the command can use to set its internals
type decodableCommand[T any, M any] interface {
	command.Command
	Decode(n *Network, msg M) error
	*T
}

// RegisterCommand register a decoder for the given command and message pair
// MT is the message type (ex: cmd_pb.CreateServiceCommand)
// CT is the command type (ex: CreateServiceCommand)
// M is the CommandMsg/command.TypedMessage implementation (ex: *cmd_pb.CreateServiceCommand)
// C is the decodableCommand/command.Command implementation (ex: *CreateServiceCommand)
//
// We only have both types specified so that we can enforce that each is a pointer type. If didn't
// enforce that the instances were pointer types, we couldn't use new to instantiate new instances.
func RegisterCommand[MT any, CT any, M CommandMsg[MT], C decodableCommand[CT, M]](managers *Managers, _ C, _ M) {
	decoder := func(commandType int32, data []byte) (command.Command, error) {
		var msg M = new(MT)
		if err := proto.Unmarshal(data, msg); err != nil {
			return nil, err
		}

		cmd := C(new(CT))
		if err := cmd.Decode(managers.network, msg); err != nil {
			return nil, err
		}
		return cmd, nil
	}

	var msg M = new(MT)
	managers.Command.Decoders.RegisterF(msg.GetCommandType(), decoder)
}
