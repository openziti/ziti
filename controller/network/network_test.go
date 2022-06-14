package network

import (
	"github.com/openziti/fabric/controller/command"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/foundation/common"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/metrics"
	"github.com/openziti/storage/boltz"
	"github.com/stretchr/testify/require"
	"runtime"
	"testing"
	"time"
)

type testConfig struct {
	ctx             *db.TestContext
	options         *Options
	metricsRegistry metrics.Registry
	versionProvider common.VersionProvider
	closeNotify     chan struct{}
}

func newTestConfig(ctx *db.TestContext) *testConfig {
	options := DefaultOptions()
	options.MinRouterCost = 0

	return &testConfig{
		ctx:             ctx,
		options:         options,
		metricsRegistry: metrics.NewRegistry("test", nil),
		versionProvider: NewVersionProviderTest(),
		closeNotify:     make(chan struct{}),
	}
}

func (self *testConfig) GetId() *identity.TokenId {
	return &identity.TokenId{Token: "test"}
}

func (self *testConfig) GetMetricsRegistry() metrics.Registry {
	return self.metricsRegistry
}

func (self *testConfig) GetOptions() *Options {
	return self.options
}

func (self *testConfig) GetCommandDispatcher() command.Dispatcher {
	return nil
}

func (self *testConfig) GetDb() boltz.Db {
	return self.ctx.GetDb()
}

func (self *testConfig) GetMetricsConfig() *metrics.Config {
	return nil
}

func (self *testConfig) GetVersionProvider() common.VersionProvider {
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

type VersionProviderTest struct {
}

func (v VersionProviderTest) Branch() string {
	return "local"
}

func (v VersionProviderTest) EncoderDecoder() common.VersionEncDec {
	return &common.StdVersionEncDec
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

func (v VersionProviderTest) AsVersionInfo() *common.VersionInfo {
	return &common.VersionInfo{
		Version:   v.Version(),
		Revision:  v.Revision(),
		BuildDate: v.BuildDate(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}

func NewVersionProviderTest() common.VersionProvider {
	return &VersionProviderTest{}
}
