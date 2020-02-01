/*
	Copyright 2019 Netfoundry, Inc.

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
	jwt2 "github.com/dgrijalva/jwt-go"
	"github.com/google/uuid"
	"github.com/netfoundry/ziti-edge/controller/config"
	"github.com/netfoundry/ziti-edge/controller/persistence"
	"github.com/netfoundry/ziti-edge/internal/cert"
	"github.com/netfoundry/ziti-edge/internal/jwt"
	"testing"
)

type TestContext struct {
	*persistence.TestContext
	handlers *Handlers
	config   *config.Config
}

func (ctx *TestContext) Generate(string, string, jwt2.MapClaims) (string, error) {
	return "I'm a very legitimate", nil
}

func (ctx *TestContext) GetHandlers() *Handlers {
	return ctx.handlers
}

func (ctx *TestContext) GetConfig() *config.Config {
	return ctx.config
}

func (ctx *TestContext) GetEnrollmentJwtGenerator() jwt.EnrollmentGenerator {
	return ctx
}

func (ctx *TestContext) GetDbProvider() persistence.DbProvider {
	return ctx.TestContext
}

func (ctx *TestContext) GetAuthRegistry() AuthRegistry {
	panic("implement me")
}

func (ctx *TestContext) GetEnrollRegistry() EnrollmentRegistry {
	panic("implement me")
}

func (ctx *TestContext) GetApiClientCsrSigner() cert.Signer {
	panic("implement me")
}

func (ctx *TestContext) GetApiServerCsrSigner() cert.Signer {
	panic("implement me")
}

func (ctx *TestContext) GetControlClientCsrSigner() cert.Signer {
	panic("implement me")
}

func (ctx *TestContext) GetHostController() HostController {
	panic("implement me")
}

func (ctx *TestContext) GetSchemas() Schemas {
	panic("implement me")
}

func (ctx *TestContext) IsEdgeRouterOnline(string) bool {
	panic("implement me")
}

func newTestContext(t *testing.T) *TestContext {
	context := &TestContext{
		TestContext: persistence.NewTestContext(t),
	}
	context.Init()
	return context
}

func (ctx *TestContext) Init() {
	ctx.TestContext.Init()
	ctx.config = &config.Config{
		Enrollment: config.Enrollment{
			EdgeRouter: config.EnrollmentOption{
				DurationMinutes: 60,
			},
		},
	}
	ctx.handlers = InitHandlers(ctx)
}

func (ctx *TestContext) Cleanup() {
	ctx.TestContext.Cleanup()
}

func (ctx *TestContext) requireNewIdentity(isAdmin bool) *Identity {
	identityType, err := ctx.handlers.IdentityType.ReadByIdOrName("Service")
	ctx.NoError(err)
	identity := &Identity{
		Name:           uuid.New().String(),
		IsAdmin:        isAdmin,
		IdentityTypeId: identityType.Id,
	}
	identity.Id, err = ctx.handlers.Identity.Create(identity)
	ctx.NoError(err)
	return identity
}

func (ctx *TestContext) requireNewService() *Service {
	service := &Service{
		EndpointAddress: "hosted:unclaimed",
		EgressRouter:    "unclaimed",
		Name:            uuid.New().String(),
	}
	var err error
	service.Id, err = ctx.handlers.Service.Create(service)
	ctx.NoError(err)
	return service
}

func (ctx *TestContext) requireNewEdgeRouter() *EdgeRouter {
	edgeRouter := &EdgeRouter{
		Name: uuid.New().String(),
	}
	var err error
	edgeRouter.Id, err = ctx.handlers.EdgeRouter.Create(edgeRouter)
	ctx.NoError(err)
	return edgeRouter
}

func (ctx *TestContext) requireNewEdgeRouterPolicy(identityRoles, edgeRouterRoles []string) *EdgeRouterPolicy {
	edgeRouterPolicy := &EdgeRouterPolicy{
		Name:            uuid.New().String(),
		IdentityRoles:   identityRoles,
		EdgeRouterRoles: edgeRouterRoles,
	}
	var err error
	edgeRouterPolicy.Id, err = ctx.handlers.EdgeRouterPolicy.Create(edgeRouterPolicy)
	ctx.NoError(err)
	return edgeRouterPolicy
}

func ss(vals ...string) []string {
	return vals
}
