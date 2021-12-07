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
	"github.com/openziti/fabric/controller/network"
	"google.golang.org/protobuf/proto"
)

type Handlers struct {
	// fabric
	Router     *network.RouterManager
	Service    *network.ServiceManager
	Terminator *network.TerminatorManager
	Command    *network.CommandManager

	// edge
	ApiSession              *ApiSessionHandler
	ApiSessionCertificate   *ApiSessionCertificateHandler
	Ca                      *CaHandler
	Config                  *ConfigHandler
	ConfigType              *ConfigTypeHandler
	EdgeRouter              *EdgeRouterHandler
	EdgeRouterPolicy        *EdgeRouterPolicyHandler
	EdgeService             *EdgeServiceHandler
	EventLog                *EventLogHandler
	ExternalJwtSigner       *ExternalJwtSignerHandler
	GeoRegion               *GeoRegionHandler
	Identity                *IdentityHandler
	IdentityType            *IdentityTypeHandler
	PolicyAdvisor           *PolicyAdvisor
	ServiceEdgeRouterPolicy *ServiceEdgeRouterPolicyHandler
	ServicePolicy           *ServicePolicyHandler
	TransitRouter           *TransitRouterHandler
	Session                 *SessionHandler
	Authenticator           *AuthenticatorHandler
	Enrollment              *EnrollmentHandler
	PostureCheck            *PostureCheckHandler
	PostureCheckType        *PostureCheckTypeHandler
	PostureResponse         *PostureResponseHandler
	Mfa                     *MfaHandler
	AuthPolicy              *AuthPolicyHandler
}

func InitHandlers(env Env) *Handlers {
	handlers := &Handlers{}

	handlers.Command = env.GetDbProvider().GetManagers().Command
	handlers.Router = env.GetDbProvider().GetManagers().Routers
	handlers.Service = env.GetDbProvider().GetManagers().Services
	handlers.Terminator = env.GetDbProvider().GetManagers().Terminators

	handlers.ApiSession = NewApiSessionHandler(env)
	handlers.ApiSessionCertificate = NewApiSessionCertificateHandler(env)
	handlers.Authenticator = NewAuthenticatorHandler(env)
	handlers.AuthPolicy = NewAuthPolicyHandler(env)
	handlers.Ca = NewCaHandler(env)
	handlers.Config = NewConfigHandler(env)
	handlers.ConfigType = NewConfigTypeHandler(env)
	handlers.EdgeRouter = NewEdgeRouterHandler(env)
	handlers.EdgeRouterPolicy = NewEdgeRouterPolicyHandler(env)
	handlers.EdgeService = NewEdgeServiceHandler(env)
	handlers.Enrollment = NewEnrollmentHandler(env)
	handlers.EventLog = NewEventLogHandler(env)
	handlers.ExternalJwtSigner = NewExternalJwtSignerHandler(env)
	handlers.GeoRegion = NewGeoRegionHandler(env)
	handlers.Identity = NewIdentityHandler(env)
	handlers.IdentityType = NewIdentityTypeHandler(env)
	handlers.PolicyAdvisor = NewPolicyAdvisor(env)
	handlers.ServiceEdgeRouterPolicy = NewServiceEdgeRouterPolicyHandler(env)
	handlers.ServicePolicy = NewServicePolicyHandler(env)
	handlers.Session = NewSessionHandler(env)
	handlers.TransitRouter = NewTransitRouterHandler(env)
	handlers.PostureCheck = NewPostureCheckHandler(env)
	handlers.PostureCheckType = NewPostureCheckTypeHandler(env)
	handlers.PostureResponse = NewPostureResponseHandler(env)
	handlers.Mfa = NewMfaHandler(env)

	RegisterCommand(env, &CreateEdgeTerminatorCmd{}, &edge_cmd_pb.CreateEdgeTerminatorCommand{})

	return handlers
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
func RegisterCommand[MT any, CT any, M network.CommandMsg[MT], C decodableCommand[CT, M]](env Env, _ C, _ M) {
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
	env.GetHostController().GetNetwork().Managers.Command.Decoders.RegisterF(msg.GetCommandType(), decoder)
}
