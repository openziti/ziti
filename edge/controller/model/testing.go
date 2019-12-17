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
	"github.com/netfoundry/ziti-edge/edge/controller/config"
	"github.com/netfoundry/ziti-edge/edge/controller/persistence"
	"github.com/netfoundry/ziti-edge/edge/internal/cert"
	"github.com/netfoundry/ziti-edge/edge/internal/jwt"
	"testing"
)

type TestContext struct {
	*persistence.TestContext
	handlers *Handlers
}

func (ctx *TestContext) GetHandlers() *Handlers {
	return ctx.handlers
}

func (ctx *TestContext) GetConfig() *config.Config {
	panic("implement me")
}

func (ctx *TestContext) GetEnrollmentJwtGenerator() jwt.EnrollmentGenerator {
	panic("implement me")
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

func (ctx *TestContext) IsEdgeRouterOnline(id string) bool {
	panic("implement me")
}

func NewTestContext(t *testing.T) *TestContext {
	return &TestContext{
		TestContext: persistence.NewTestContext(t),
	}
}

func (ctx *TestContext) Init() {
	ctx.TestContext.Init()
	ctx.handlers = InitHandlers(ctx)
}

func (ctx *TestContext) Cleanup() {
	ctx.TestContext.Cleanup()
}

func (ctx *TestContext) requireNewIdentity(name string, isAdmin bool) *Identity {
	identity := &Identity{
		Name:    name,
		IsAdmin: isAdmin,
	}
	var err error
	identity.Id, err = ctx.handlers.Identity.HandleCreate(identity)
	ctx.NoError(err)
	return identity
}

func (ctx *TestContext) requireNewService(name string) *Service {
	service := &Service{
		EndpointAddress: "hosted:unclaimed",
		EgressRouter:    "unclaimed",
		Name:            name,
		DnsHostname:     name,
		DnsPort:         0,
	}
	var err error
	service.Id, err = ctx.handlers.Service.HandleCreate(service)
	ctx.NoError(err)
	return service
}
