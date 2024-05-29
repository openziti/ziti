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
	"crypto/tls"
	"crypto/x509"
	"github.com/openziti/channel/v3"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/controller/models"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/openziti/identity"
	"github.com/openziti/metrics"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/common/cert"
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/command"
	"github.com/openziti/ziti/controller/config"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/event"
	"github.com/openziti/ziti/controller/jwtsigner"
)

var _ Env = &TestContext{}

type TestContext struct {
	*db.TestContext
	managers        *Managers
	config          *config.Config
	metricsRegistry metrics.Registry
	closeNotify     chan struct{}
	dispatcher      command.Dispatcher
	eventDispatcher event.Dispatcher
}

func (ctx *TestContext) GetId() string {
	return ctx.config.Id.Token
}

func (ctx *TestContext) GetEnrollmentJwtSigner() (jwtsigner.Signer, error) {
	return ctx, nil
}

func (ctx *TestContext) GetEventDispatcher() event.Dispatcher {
	return ctx.eventDispatcher
}

func (self *TestContext) GetCloseNotifyChannel() <-chan struct{} {
	return self.closeNotify
}

func (ctx *TestContext) ValidateAccessToken(token string) (*common.AccessClaims, error) {
	panic("implement me")
}

func (ctx *TestContext) ValidateServiceAccessToken(token string, apiSessionId *string) (*common.ServiceAccessClaims, error) {
	panic("implement me")
}

func (ctx *TestContext) OidcIssuer() string {
	panic("implement me")
}

func (ctx *TestContext) RootIssuer() string {
	panic("implement me")
}

func (ctx *TestContext) GetPeerControllerAddresses() []string {
	return nil
}

func (ctx *TestContext) SigningMethod() jwt.SigningMethod {
	return nil
}

func (ctx *TestContext) KeyId() string {
	return "123-test-context"
}

func (ctx *TestContext) JwtSignerKeyFunc(*jwt.Token) (interface{}, error) {
	tlsCert, _, _ := ctx.GetServerCert()
	return tlsCert.Leaf.PublicKey, nil
}

func (ctx *TestContext) GetServerCert() (*tls.Certificate, string, jwt.SigningMethod) {
	return nil, "", nil
}

func (ctx *TestContext) HandleServiceUpdatedEventForIdentityId(string) {}

func (ctx *TestContext) Generate(jwt.Claims) (string, error) {
	return "I'm a very legitimate claim", nil
}

func (ctx *TestContext) GetManagers() *Managers {
	return ctx.managers
}

func (ctx *TestContext) GetConfig() *config.Config {
	return ctx.config
}

func (ctx *TestContext) GetServerJwtSigner() jwtsigner.Signer {
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
	return nil
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

func (self *TestContext) GetApiAddresses() (map[string][]event.ApiAddress, []byte) {
	return nil, nil
}

func (self *TestContext) GetRaftInfo() (string, string, string) {
	return "testaddr", "testid", "testversion"
}

func (self *TestContext) GetPeerSigners() []*x509.Certificate {
	return nil
}

func (self *TestContext) Identity() identity.Identity {
	return &identity.TokenId{Token: "test"}
}

func (self *TestContext) Shutdown() {
	close(self.closeNotify)
}

func (self *TestContext) Stop() {
	close(self.closeNotify)
}

func (self *TestContext) GetCommandDispatcher() command.Dispatcher {
	return self.dispatcher
}

func (self *TestContext) AddRouterPresenceHandler(RouterPresenceHandler) {}

func NewTestContext(t testing.TB) *TestContext {
	fabricTestContext := db.NewTestContext(t)
	ctx := &TestContext{
		TestContext:     fabricTestContext,
		metricsRegistry: metrics.NewRegistry("test", nil),
		closeNotify:     make(chan struct{}),
		dispatcher: &command.LocalDispatcher{
			EncodeDecodeCommands: true,
			Limiter:              command.NoOpRateLimiter{},
		},
		eventDispatcher: event.DispatcherMock{},
	}

	ctx.TestContext.Init()

	ctx.config = &config.Config{
		Id: &identity.TokenId{
			Token: "test",
		},
		Network: config.DefaultNetworkConfig(),
		Edge: &config.EdgeConfig{
			Enrollment: config.Enrollment{
				EdgeRouter: config.EnrollmentOption{
					Duration: 60 * time.Second,
				},
			},
		},
	}
	ctx.managers = NewManagers()
	ctx.managers.Init(ctx)

	return ctx
}

func (ctx *TestContext) Cleanup() {
	ctx.Stop()
	ctx.TestContext.Cleanup()
}

func (ctx *TestContext) requireNewIdentity(isAdmin bool) *Identity {
	newIdentity := &Identity{
		Name:           eid.New(),
		IsAdmin:        isAdmin,
		IdentityTypeId: db.DefaultIdentityType,
	}
	ctx.NoError(ctx.managers.Identity.Create(newIdentity, change.New()))
	return newIdentity
}

func (ctx *TestContext) requireNewService(cfgs ...string) *EdgeService {
	service := &EdgeService{
		Name:    eid.New(),
		Configs: cfgs,
	}
	ctx.NoError(ctx.managers.EdgeService.Create(service, change.New()))
	return service
}

func (ctx *TestContext) requireNewConfig(configTypeName string, data map[string]any) *Config {
	cfgType, err := ctx.managers.ConfigType.ReadByName(configTypeName)
	ctx.NoError(err)

	cfg := &Config{
		Name:   eid.New(),
		TypeId: cfgType.Id,
		Data:   data,
	}
	ctx.NoError(ctx.managers.Config.Create(cfg, change.New()))
	return cfg
}

func (ctx *TestContext) requireNewEdgeRouter() *EdgeRouter {
	edgeRouter := &EdgeRouter{
		Name: eid.New(),
	}
	ctx.NoError(ctx.managers.EdgeRouter.Create(edgeRouter, change.New()))
	return edgeRouter
}

func (ctx *TestContext) requireNewApiSession(identity *Identity) *ApiSession {
	entity := &ApiSession{
		Token:          uuid.NewString(),
		IdentityId:     identity.Id,
		Identity:       identity,
		LastActivityAt: time.Now(),
	}
	_, err := ctx.managers.ApiSession.Create(nil, entity, nil)
	ctx.NoError(err)
	return entity
}

func (ctx *TestContext) requireNewSession(apiSession *ApiSession, serviceId string, sessionType string) *Session {
	entity := &Session{
		Token:        uuid.NewString(),
		IdentityId:   apiSession.IdentityId,
		ApiSessionId: apiSession.Id,
		ServiceId:    serviceId,
		Type:         sessionType,
	}
	_, err := ctx.managers.Session.Create(entity, change.New())
	ctx.NoError(err)
	return entity
}

func (ctx *TestContext) requireNewServicePolicy(policyType string, identityRoles, serviceRoles []string) *ServicePolicy {
	policy := &ServicePolicy{
		Name:          eid.New(),
		Semantic:      db.SemanticAllOf,
		IdentityRoles: identityRoles,
		ServiceRoles:  serviceRoles,
		PolicyType:    policyType,
	}
	ctx.NoError(ctx.managers.ServicePolicy.Create(policy, change.New()))
	return policy
}

func (ctx *TestContext) requireNewEdgeRouterPolicy(identityRoles, edgeRouterRoles []string) *EdgeRouterPolicy {
	policy := &EdgeRouterPolicy{
		Name:            eid.New(),
		Semantic:        db.SemanticAllOf,
		IdentityRoles:   identityRoles,
		EdgeRouterRoles: edgeRouterRoles,
	}
	ctx.NoError(ctx.managers.EdgeRouterPolicy.Create(policy, change.New()))
	return policy
}

func (ctx *TestContext) requireNewServiceNewEdgeRouterPolicy(serviceRoles, edgeRouterRoles []string) *ServiceEdgeRouterPolicy {
	policy := &ServiceEdgeRouterPolicy{
		Name:            eid.New(),
		Semantic:        db.SemanticAllOf,
		ServiceRoles:    serviceRoles,
		EdgeRouterRoles: edgeRouterRoles,
	}
	ctx.NoError(ctx.managers.ServiceEdgeRouterPolicy.Create(policy, change.New()))
	return policy
}

func ss(vals ...string) []string {
	return vals
}

func NewTestLink(id string, src, dst *Router) *Link {
	l := newLink(id, "tls", "tcp:localhost:1234", 0)
	l.Src = src
	l.DstId = dst.Id
	l.Dst.Store(dst)
	src.Connected.Store(true)
	dst.Connected.Store(true)
	return l
}

func NewRouterForTest(id string, fingerprint string, advLstnr transport.Address, ctrl channel.Channel, cost uint16, noTraversal bool) *Router {
	r := &Router{
		BaseEntity:  models.BaseEntity{Id: id},
		Name:        id,
		Fingerprint: &fingerprint,
		Control:     ctrl,
		Cost:        cost,
		NoTraversal: noTraversal,
	}
	if advLstnr != nil {
		r.AddLinkListener(advLstnr.String(), advLstnr.Type(), []string{"Cost Tag"}, []string{"default"})
	}
	return r
}
