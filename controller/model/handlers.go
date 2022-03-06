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
	"github.com/openziti/fabric/controller/network"
)

type Handlers struct {
	// fabric
	Router     *network.RouterController
	Service    *network.ServiceController
	Terminator *network.TerminatorController

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

	handlers.Router = env.GetDbProvider().GetControllers().Routers
	handlers.Service = env.GetDbProvider().GetControllers().Services
	handlers.Terminator = env.GetDbProvider().GetControllers().Terminators

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

	return handlers
}
