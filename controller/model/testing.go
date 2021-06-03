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
	jwt2 "github.com/dgrijalva/jwt-go"
	"github.com/openziti/edge/controller/config"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/eid"
	"github.com/openziti/edge/internal/cert"
	"github.com/openziti/edge/internal/jwtsigner"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/foundation/metrics"
	"testing"
	"time"
)

var _ Env = &TestContext{}

var _ HostController = &testHostController{}

type testHostController struct {
	closeNotify chan struct{}
}

func (t testHostController) GetNetwork() *network.Network {
	return nil
}

func (t testHostController) Shutdown() {
	close(t.closeNotify)
}

func (t testHostController) GetCloseNotifyChannel() <-chan struct{} {
	return t.closeNotify
}

func (t testHostController) Stop() {
	close(t.closeNotify)
}

type TestContext struct {
	*persistence.TestContext
	handlers        *Handlers
	config          *config.Config
	metricsRegistry metrics.Registry
	hostController  *testHostController
}

func (ctx *TestContext) HandleServiceUpdatedEventForIdentityId(identityId string) {}

func (ctx *TestContext) Generate(string, string, jwt2.MapClaims) (string, error) {
	return "I'm a very legitimate claim", nil
}

func (ctx *TestContext) GetHandlers() *Handlers {
	return ctx.handlers
}

func (ctx *TestContext) GetConfig() *config.Config {
	return ctx.config
}

func (ctx *TestContext) GetJwtSigner() jwtsigner.Signer {
	return ctx
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
	return ctx.hostController
}

func (ctx *TestContext) GetSchemas() Schemas {
	panic("implement me")
}

func (ctx *TestContext) IsEdgeRouterOnline(string) bool {
	panic("implement me")
}

func (ctx *TestContext) GetMetricsRegistry() metrics.Registry {
	return ctx.metricsRegistry
}

func (ctx *TestContext) GetFingerprintGenerator() cert.FingerprintGenerator {
	return nil
}

func newTestContext(t *testing.T) *TestContext {
	context := &TestContext{
		TestContext:     persistence.NewTestContext(t),
		metricsRegistry: metrics.NewRegistry("test", nil),
		hostController: &testHostController{
			closeNotify: make(chan struct{}),
		},
	}
	context.Init()
	return context
}

func (ctx *TestContext) Init() {
	ctx.TestContext.Init()
	ctx.config = &config.Config{
		Enrollment: config.Enrollment{
			EdgeRouter: config.EnrollmentOption{
				Duration: 60 * time.Second,
			},
		},
	}
	ctx.handlers = InitHandlers(ctx)
}

func (ctx *TestContext) Cleanup() {
	if ctx.hostController != nil {
		ctx.hostController.Stop()
	}
	ctx.TestContext.Cleanup()
}

func (ctx *TestContext) requireNewIdentity(isAdmin bool) *Identity {
	identityType, err := ctx.handlers.IdentityType.ReadByIdOrName("Service")
	ctx.NoError(err)
	identity := &Identity{
		Name:           eid.New(),
		IsAdmin:        isAdmin,
		IdentityTypeId: identityType.Id,
	}
	identity.Id, err = ctx.handlers.Identity.Create(identity)
	ctx.NoError(err)
	return identity
}

func (ctx *TestContext) requireNewService() *Service {
	service := &Service{
		Name: eid.New(),
	}
	var err error
	service.Id, err = ctx.handlers.EdgeService.Create(service)
	ctx.NoError(err)
	return service
}

func (ctx *TestContext) requireNewEdgeRouter() *EdgeRouter {
	edgeRouter := &EdgeRouter{
		Name: eid.New(),
	}
	var err error
	edgeRouter.Id, err = ctx.handlers.EdgeRouter.Create(edgeRouter)
	ctx.NoError(err)
	return edgeRouter
}

func (ctx *TestContext) requireNewEdgeRouterPolicy(identityRoles, edgeRouterRoles []string) *EdgeRouterPolicy {
	policy := &EdgeRouterPolicy{
		Name:            eid.New(),
		IdentityRoles:   identityRoles,
		EdgeRouterRoles: edgeRouterRoles,
	}
	var err error
	policy.Id, err = ctx.handlers.EdgeRouterPolicy.Create(policy)
	ctx.NoError(err)
	return policy
}

func (ctx *TestContext) requireNewServiceNewEdgeRouterPolicy(serviceRoles, edgeRouterRoles []string) *ServiceEdgeRouterPolicy {
	policy := &ServiceEdgeRouterPolicy{
		Name:            eid.New(),
		ServiceRoles:    serviceRoles,
		EdgeRouterRoles: edgeRouterRoles,
	}
	var err error
	policy.Id, err = ctx.handlers.ServiceEdgeRouterPolicy.Create(policy)
	ctx.NoError(err)
	return policy
}

func ss(vals ...string) []string {
	return vals
}
