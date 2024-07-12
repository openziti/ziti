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
	"github.com/openziti/ziti/common/pb/edge_cmd_pb"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/command"
	"github.com/openziti/ziti/controller/fields"
	"github.com/openziti/ziti/controller/ioc"
	"github.com/openziti/ziti/controller/models"
	"google.golang.org/protobuf/proto"
)

const (
	CreateDecoder = "CreateDecoder"
	UpdateDecoder = "UpdateDecoder"
	DeleteDecoder = "DeleteDecoder"
)

type Managers struct {
	// command
	Registry   ioc.Registry
	Dispatcher command.Dispatcher

	// fabric
	Circuit    *CircuitManager
	Command    *CommandManager
	Link       *LinkManager
	Router     *RouterManager
	Service    *ServiceManager
	Terminator *TerminatorManager

	// edge
	ApiSession              *ApiSessionManager
	ApiSessionCertificate   *ApiSessionCertificateManager
	Ca                      *CaManager
	Config                  *ConfigManager
	ConfigType              *ConfigTypeManager
	Controller              *ControllerManager
	EdgeRouter              *EdgeRouterManager
	EdgeRouterPolicy        *EdgeRouterPolicyManager
	EdgeService             *EdgeServiceManager
	ExternalJwtSigner       *ExternalJwtSignerManager
	Identity                *IdentityManager
	IdentityType            *IdentityTypeManager
	PolicyAdvisor           *PolicyAdvisor
	ServiceEdgeRouterPolicy *ServiceEdgeRouterPolicyManager
	ServicePolicy           *ServicePolicyManager
	Revocation              *RevocationManager
	TransitRouter           *TransitRouterManager
	Session                 *SessionManager
	Authenticator           *AuthenticatorManager
	Enrollment              *EnrollmentManager
	PostureCheck            *PostureCheckManager
	PostureCheckType        *PostureCheckTypeManager
	PostureResponse         *PostureResponseManager
	Mfa                     *MfaManager
	AuthPolicy              *AuthPolicyManager
}

func NewManagers() *Managers {
	return &Managers{
		Registry: ioc.NewRegistry(),
	}
}

func (managers *Managers) Init(env Env) *Managers {
	managers.Dispatcher = env.GetCommandDispatcher()
	managers.Circuit = NewCircuitController()
	managers.Command = newCommandManager(env, managers.Registry)
	managers.Link = NewLinkManager(env)
	managers.Router = newRouterManager(env)
	managers.Service = newServiceManager(env)
	managers.Terminator = newTerminatorManager(env)

	managers.ApiSession = NewApiSessionManager(env)
	managers.ApiSessionCertificate = NewApiSessionCertificateManager(env)
	managers.Authenticator = NewAuthenticatorManager(env)
	managers.AuthPolicy = NewAuthPolicyManager(env)
	managers.Ca = NewCaManager(env)
	managers.Config = NewConfigManager(env)
	managers.ConfigType = NewConfigTypeManager(env)
	managers.Controller = NewControllerManager(env)
	managers.EdgeRouter = NewEdgeRouterManager(env)
	managers.EdgeRouterPolicy = NewEdgeRouterPolicyManager(env)
	managers.EdgeService = NewEdgeServiceManager(env)
	managers.Enrollment = NewEnrollmentManager(env)
	managers.ExternalJwtSigner = NewExternalJwtSignerManager(env)
	managers.Identity = NewIdentityManager(env)
	managers.IdentityType = NewIdentityTypeManager(env)
	managers.PolicyAdvisor = NewPolicyAdvisor(env)
	managers.Revocation = NewRevocationManager(env)
	managers.ServiceEdgeRouterPolicy = NewServiceEdgeRouterPolicyManager(env)
	managers.ServicePolicy = NewServicePolicyManager(env)
	managers.Session = NewSessionManager(env)
	managers.TransitRouter = NewTransitRouterManager(env)
	managers.PostureCheck = NewPostureCheckManager(env)
	managers.PostureCheckType = NewPostureCheckTypeManager(env)
	managers.PostureResponse = NewPostureResponseManager(env)
	managers.Mfa = NewMfaManager(env)

	RegisterCommand(env, &CreateEdgeTerminatorCmd{}, &edge_cmd_pb.CreateEdgeTerminatorCommand{})
	managers.Command.registerGenericCommands()

	return managers
}

// decodableCommand is a Command which knows how to decode itself from the given message type
//
// T is the type of the command. We want to enforce that the command is a pointer type so we can
// use new(T) to create new instances of it
// M is the message type that the command can use to set its internals
type decodableCommand[T any, M any] interface {
	command.Command
	Decode(env Env, msg M) error
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
func RegisterCommand[MT any, CT any, M CommandMsg[MT], C decodableCommand[CT, M]](env Env, _ C, _ M) {
	decoder := func(commandType int32, data []byte) (command.Command, error) {
		var msg M = new(MT)
		if err := proto.Unmarshal(data, msg); err != nil {
			return nil, err
		}

		cmd := C(new(CT))
		if err := cmd.Decode(env, msg); err != nil {
			return nil, err
		}
		return cmd, nil
	}

	var msg M = new(MT)
	env.GetManagers().Command.Decoders.RegisterF(msg.GetCommandType(), decoder)
}

type createDecoderF func(cmd *cmd_pb.CreateEntityCommand) (command.Command, error)

func RegisterCreateDecoder[T models.Entity](env Env, creator command.EntityCreator[T]) {
	entityType := creator.GetEntityTypeId()
	env.GetManagers().Registry.RegisterSingleton(entityType+CreateDecoder, createDecoderF(func(cmd *cmd_pb.CreateEntityCommand) (command.Command, error) {
		entity, err := creator.Unmarshall(cmd.EntityData)
		if err != nil {
			return nil, err
		}
		return &command.CreateEntityCommand[T]{
			Context: change.FromProtoBuf(cmd.Ctx),
			Entity:  entity,
			Creator: creator,
			Flags:   cmd.Flags,
		}, nil
	}))
}

type updateDecoderF func(cmd *cmd_pb.UpdateEntityCommand) (command.Command, error)

func RegisterUpdateDecoder[T models.Entity](env Env, updater command.EntityUpdater[T]) {
	entityType := updater.GetEntityTypeId()
	env.GetManagers().Registry.RegisterSingleton(entityType+UpdateDecoder, updateDecoderF(func(cmd *cmd_pb.UpdateEntityCommand) (command.Command, error) {
		entity, err := updater.Unmarshall(cmd.EntityData)
		if err != nil {
			return nil, err
		}
		return &command.UpdateEntityCommand[T]{
			Context:       change.FromProtoBuf(cmd.Ctx),
			Entity:        entity,
			Updater:       updater,
			UpdatedFields: fields.SliceToUpdatedFields(cmd.UpdatedFields),
			Flags:         cmd.Flags,
		}, nil
	}))
}

type deleteDecoderF func(cmd *cmd_pb.DeleteEntityCommand) (command.Command, error)

func RegisterDeleteDecoder(env Env, deleter command.EntityDeleter) {
	entityType := deleter.GetEntityTypeId()
	env.GetManagers().Registry.RegisterSingleton(entityType+DeleteDecoder, deleteDecoderF(func(cmd *cmd_pb.DeleteEntityCommand) (command.Command, error) {
		return &command.DeleteEntityCommand{
			Context: change.FromProtoBuf(cmd.Ctx),
			Deleter: deleter,
			Id:      cmd.EntityId,
		}, nil
	}))
}

func RegisterManagerDecoder[T models.Entity](env Env, ctrl command.EntityManager[T]) {
	RegisterCreateDecoder[T](env, ctrl)
	RegisterUpdateDecoder[T](env, ctrl)
	RegisterDeleteDecoder(env, ctrl)
}
