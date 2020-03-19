/*
	Copyright 2020 NetFoundry, Inc.

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

import "github.com/netfoundry/ziti-fabric/controller/network"

type Handlers struct {
	// fabric
	Router     *network.RouterController
	Service    *network.ServiceController
	Terminator *network.TerminatorController

	// edge
	ApiSession              *ApiSessionHandler
	Ca                      *CaHandler
	Config                  *ConfigHandler
	ConfigType              *ConfigTypeHandler
	EdgeRouter              *EdgeRouterHandler
	EdgeRouterPolicy        *EdgeRouterPolicyHandler
	EdgeService             *EdgeServiceHandler
	EventLog                *EventLogHandler
	GeoRegion               *GeoRegionHandler
	Identity                *IdentityHandler
	IdentityType            *IdentityTypeHandler
	ServiceEdgeRouterPolicy *ServiceEdgeRouterPolicyHandler
	ServicePolicy           *ServicePolicyHandler
	Session                 *SessionHandler

	Authenticator *AuthenticatorHandler
	Enrollment    *EnrollmentHandler
}

func InitHandlers(env Env) *Handlers {
	handlers := &Handlers{}

	handlers.Router = env.GetDbProvider().GetControllers().Routers
	handlers.Service = env.GetDbProvider().GetControllers().Services
	handlers.Terminator = env.GetDbProvider().GetControllers().Terminators

	handlers.ApiSession = NewApiSessionHandler(env)
	handlers.Authenticator = NewAuthenticatorHandler(env)
	handlers.Ca = NewCaHandler(env)
	handlers.Config = NewConfigHandler(env)
	handlers.ConfigType = NewConfigTypeHandler(env)
	handlers.EdgeRouter = NewEdgeRouterHandler(env)
	handlers.EdgeRouterPolicy = NewEdgeRouterPolicyHandler(env)
	handlers.Enrollment = NewEnrollmentHandler(env)
	handlers.EventLog = NewEventLogHandler(env)
	handlers.GeoRegion = NewGeoRegionHandler(env)
	handlers.Identity = NewIdentityHandler(env)
	handlers.IdentityType = NewIdentityTypeHandler(env)
	handlers.EdgeService = NewEdgeServiceHandler(env)
	handlers.ServiceEdgeRouterPolicy = NewServiceEdgeRouterPolicyHandler(env)
	handlers.ServicePolicy = NewServicePolicyHandler(env)
	handlers.Session = NewSessionHandler(env)

	return handlers
}
