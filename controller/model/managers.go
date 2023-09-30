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
	"github.com/openziti/ziti/common/pb/edge_cmd_pb"
	"github.com/openziti/ziti/controller/command"
	"github.com/openziti/ziti/controller/network"
	"google.golang.org/protobuf/proto"
)

type Managers struct {
	// fabric
	Router     *network.RouterManager
	Service    *network.ServiceManager
	Terminator *network.TerminatorManager
	Command    *network.CommandManager

	// edge
	ApiSession              *ApiSessionManager
	ApiSessionCertificate   *ApiSessionCertificateManager
	Ca                      *CaManager
	Config                  *ConfigManager
	ConfigType              *ConfigTypeManager
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

func InitEntityManagers(env Env) *Managers {
	managers := &Managers{}

	managers.Command = env.GetDbProvider().GetManagers().Command
	managers.Router = env.GetDbProvider().GetManagers().Routers
	managers.Service = env.GetDbProvider().GetManagers().Services
	managers.Terminator = env.GetDbProvider().GetManagers().Terminators

	managers.ApiSession = NewApiSessionManager(env)
	managers.ApiSessionCertificate = NewApiSessionCertificateManager(env)
	managers.Authenticator = NewAuthenticatorManager(env)
	managers.AuthPolicy = NewAuthPolicyManager(env)
	managers.Ca = NewCaManager(env)
	managers.Config = NewConfigManager(env)
	managers.ConfigType = NewConfigTypeManager(env)
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
