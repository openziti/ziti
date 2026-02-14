package network

import (
	"math"
	"runtime"
	"testing"
	"time"

	"github.com/openziti/ziti/v2/controller/config"
	"github.com/openziti/ziti/v2/controller/event"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/pkg/errors"

	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/identity"
	"github.com/openziti/metrics"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/transport/v2/tcp"
	"github.com/openziti/ziti/v2/common/logcontext"
	"github.com/openziti/ziti/v2/controller/command"
	"github.com/openziti/ziti/v2/controller/db"
	"github.com/openziti/ziti/v2/controller/models"
	"github.com/openziti/ziti/v2/controller/xt"
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

func TestMatchesTerminatorInstanceId(t *testing.T) {
	tests := []struct {
		name       string
		terminator string
		requested  string
		expected   bool
	}{
		// Exact matches
		{"exact match", "node1.ziti.internal", "node1.ziti.internal", true},
		{"case insensitive exact", "Node1.Ziti.Internal", "node1.ziti.internal", true},
		{"no match", "node1.ziti.internal", "node2.ziti.internal", false},

		// Empty cases (empty only matches empty, preserving backward compatibility)
		{"empty terminator vs non-empty request", "", "anything", false},
		{"empty request vs non-empty terminator", "node1", "", false},
		{"both empty", "", "", true},

		// Wildcard exact base match
		{"wildcard exact base", "*:node1.ziti.internal", "node1.ziti.internal", true},
		{"wildcard exact base case insensitive", "*:Node1.Ziti.Internal", "node1.ziti.internal", true},

		// Wildcard subdomain matches
		{"wildcard single subdomain", "*:node1.ziti.internal", "acme.node1.ziti.internal", true},
		{"wildcard multi-level subdomain", "*:node1.ziti.internal", "foo.bar.node1.ziti.internal", true},
		{"wildcard case insensitive subdomain", "*:Node1.Ziti.Internal", "ACME.node1.ziti.internal", true},

		// Wildcard non-matches
		{"wildcard wrong domain", "*:node1.ziti.internal", "node1.example.com", false},
		{"wildcard partial name (not proper subdomain)", "*:node1.ziti.internal", "mynode1.ziti.internal", false},
		{"wildcard empty request", "*:node1.ziti.internal", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesTerminatorInstanceId(tt.terminator, tt.requested)
			assert.Equal(t, tt.expected, result, "matchesTerminatorInstanceId(%q, %q)", tt.terminator, tt.requested)
		})
	}
}

func TestWildcardBaseLen(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"empty string", "", 0},
		{"exact id", "node1.ziti.internal", math.MaxInt},
		{"wildcard short", "*:ziti.internal", 13},
		{"wildcard long", "*:node1.region1.ziti.internal", 27},
		{"wildcard single char", "*:a", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wildcardBaseLen(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
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
