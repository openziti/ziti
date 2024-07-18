package network

import (
	"github.com/openziti/ziti/controller/config"
	"github.com/openziti/ziti/controller/event"
	"github.com/openziti/ziti/controller/model"
	"github.com/pkg/errors"
	"runtime"
	"testing"
	"time"

	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/identity"
	"github.com/openziti/metrics"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/transport/v2/tcp"
	"github.com/openziti/ziti/common/logcontext"
	"github.com/openziti/ziti/controller/command"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/models"
	"github.com/openziti/ziti/controller/xt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testConfig struct {
	ctx             *db.TestContext
	options         *config.NetworkConfig
	metricsRegistry metrics.Registry
	versionProvider versions.VersionProvider
	closeNotify     chan struct{}
}

func (self *testConfig) RenderJsonConfig() (string, error) {
	panic(errors.New("not implemented"))
}

func newTestConfig(ctx *model.TestContext) *testConfig {
	options := config.DefaultNetworkConfig()
	options.MinRouterCost = 0

	closeNotify := make(chan struct{})
	return &testConfig{
		ctx:             ctx.TestContext,
		options:         options,
		metricsRegistry: metrics.NewRegistry("test", nil),
		versionProvider: NewVersionProviderTest(),
		closeNotify:     closeNotify,
	}
}

func (self *testConfig) GetEventDispatcher() event.Dispatcher {
	return event.DispatcherMock{}
}

func (self *testConfig) GetId() *identity.TokenId {
	return &identity.TokenId{Token: "test"}
}

func (self *testConfig) GetMetricsRegistry() metrics.Registry {
	return self.metricsRegistry
}

func (self *testConfig) GetOptions() *config.NetworkConfig {
	return self.options
}

func (self *testConfig) GetCommandDispatcher() command.Dispatcher {
	return &command.LocalDispatcher{
		Limiter: command.NoOpRateLimiter{},
	}
}

func (self *testConfig) GetDb() boltz.Db {
	return self.ctx.GetDb()
}

func (self *testConfig) GetVersionProvider() versions.VersionProvider {
	return self.versionProvider
}

func (self *testConfig) GetCloseNotify() <-chan struct{} {
	return self.closeNotify
}

func TestNetwork_parseServiceAndIdentity(t *testing.T) {
	req := require.New(t)
	instanceId, serviceId := parseInstanceIdAndService("hello")
	req.Equal("", instanceId)
	req.Equal("hello", serviceId)

	instanceId, serviceId = parseInstanceIdAndService("@hello")
	req.Equal("", instanceId)
	req.Equal("hello", serviceId)

	instanceId, serviceId = parseInstanceIdAndService("a@hello")
	req.Equal("a", instanceId)
	req.Equal("hello", serviceId)

	instanceId, serviceId = parseInstanceIdAndService("bar@hello")
	req.Equal("bar", instanceId)
	req.Equal("hello", serviceId)

	instanceId, serviceId = parseInstanceIdAndService("@@hello")
	req.Equal("", instanceId)
	req.Equal("@hello", serviceId)

	instanceId, serviceId = parseInstanceIdAndService("a@@hello")
	req.Equal("a", instanceId)
	req.Equal("@hello", serviceId)

	instanceId, serviceId = parseInstanceIdAndService("a@foo@hello")
	req.Equal("a", instanceId)
	req.Equal("foo@hello", serviceId)
}

func TestCreateCircuit(t *testing.T) {
	ctx := model.NewTestContext(t)
	defer ctx.Cleanup()

	config := newTestConfig(ctx)
	defer close(config.closeNotify)

	network, err := NewNetwork(config, ctx)
	assert.Nil(t, err)

	addr := "tcp:0.0.0.0:0"
	transportAddr, err := tcp.AddressParser{}.Parse(addr)
	assert.Nil(t, err)

	r0 := model.NewRouterForTest("r0", "", transportAddr, nil, 0, false)

	svc := &model.Service{
		BaseEntity:         models.BaseEntity{Id: "svc"},
		Name:               "svc",
		TerminatorStrategy: "smartrouting",
	}

	/*
		Terminators: []*Terminator{
		{
		},
	*/
	lc := logcontext.NewContext()
	params := newCircuitParams(svc, r0)
	_, _, _, _, cerr := network.selectPath(params, svc, "", lc)
	assert.Error(t, cerr)
	assert.Equal(t, CircuitFailureNoTerminators, cerr.Cause())

	svc.Terminators = []*model.Terminator{
		{
			BaseEntity: models.BaseEntity{Id: "t0"},
			Service:    "svc",
			Router:     "r0",
			Binding:    "transport",
			Address:    "tcp:localhost:1001",
			InstanceId: "",
			Precedence: xt.Precedences.Default,
		},
	}

	_, _, _, _, cerr = network.selectPath(params, svc, "", lc)
	assert.Error(t, cerr)
	assert.Equal(t, CircuitFailureNoOnlineTerminators, cerr.Cause())

	network.Router.MarkConnected(r0)
	_, _, _, _, cerr = network.selectPath(params, svc, "", lc)
	assert.NoError(t, cerr)

	_, _, _, _, cerr = network.selectPath(params, svc, "test", lc)
	assert.Error(t, cerr)
	assert.Equal(t, CircuitFailureNoTerminators, cerr.Cause())
}

type VersionProviderTest struct {
}

func (v VersionProviderTest) EncoderDecoder() versions.VersionEncDec {
	return &versions.StdVersionEncDec
}

func (v VersionProviderTest) Version() string {
	return "v0.0.0"
}

func (v VersionProviderTest) BuildDate() string {
	return time.Now().String()
}

func (v VersionProviderTest) Revision() string {
	return ""
}

func (v VersionProviderTest) AsVersionInfo() *versions.VersionInfo {
	return &versions.VersionInfo{
		Version:   v.Version(),
		Revision:  v.Revision(),
		BuildDate: v.BuildDate(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}

func NewVersionProviderTest() versions.VersionProvider {
	return &VersionProviderTest{}
}

func newCircuitParams(service *model.Service, router *model.Router) model.CreateCircuitParams {
	return testCreateCircuitParams{
		svc:    service,
		router: router,
	}
}

type testCreateCircuitParams struct {
	svc    *model.Service
	router *model.Router
}

func (t testCreateCircuitParams) GetServiceId() string {
	return t.svc.Id
}

func (t testCreateCircuitParams) GetSourceRouter() *model.Router {
	return t.router
}

func (t testCreateCircuitParams) GetClientId() *identity.TokenId {
	return nil
}

func (t testCreateCircuitParams) GetCircuitTags(terminator xt.CostedTerminator) map[string]string {
	return nil
}

func (t testCreateCircuitParams) GetLogContext() logcontext.Context {
	return logcontext.NewContext()
}

func (t testCreateCircuitParams) GetDeadline() time.Time {
	return time.Now().Add(time.Second)
}
