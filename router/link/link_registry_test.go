/*
	(c) Copyright NetFoundry Inc.

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

package link

import (
	"testing"
	"time"

	"github.com/openziti/channel/v4"
	"github.com/openziti/foundation/v2/goroutines"
	"github.com/openziti/identity"
	"github.com/openziti/metrics"
	"github.com/openziti/sdk-golang/xgress"
	"github.com/openziti/ziti/v2/common/ctrlchan"
	"github.com/openziti/ziti/v2/common/inspect"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/controller/idgen"
	"github.com/openziti/ziti/v2/router/env"
	"github.com/openziti/ziti/v2/router/xlink"
	"github.com/stretchr/testify/require"
)

type testEnv struct {
	metricsRegistry metrics.UsageRegistry
	closeNotify     chan struct{}
	ctrls           env.NetworkControllers
	config          *env.Config
}

func (self *testEnv) GetRouterId() *identity.TokenId {
	return &identity.TokenId{
		Token: "test",
	}
}

func (self *testEnv) GetChannelHeaders() (channel.Headers, error) {
	return channel.Headers{}, nil
}

func (self *testEnv) GetConfig() *env.Config {
	return self.config
}

func (self *testEnv) GetCtrlChannelBindHandler() channel.BindHandler {
	return channel.BindHandlerF(func(binding channel.Binding) error {
		return nil
	})
}

func (self *testEnv) NotifyOfReconnect(ch ctrlchan.CtrlChannel) {
}

func (self *testEnv) GetNetworkControllers() env.NetworkControllers {
	return self.ctrls
}

func (self *testEnv) GetXlinkDialers() []xlink.Dialer {
	panic("implement me")
}

func (self *testEnv) GetCloseNotify() <-chan struct{} {
	return self.closeNotify
}

func (self *testEnv) GetLinkDialerPool() goroutines.Pool {
	panic("implement me")
}

func (self *testEnv) GetRateLimiterPool() goroutines.Pool {
	panic("implement me")
}

func (self *testEnv) GetMetricsRegistry() metrics.UsageRegistry {
	return self.metricsRegistry
}

type testLink struct {
	id     string
	key    string
	destId string
}

func (self *testLink) GetDestinationType() string {
	return "link"
}

func (self *testLink) Id() string {
	return self.id
}

func (self *testLink) SendPayload(payload *xgress.Payload, timeout time.Duration, payloadType xgress.PayloadType) error {
	panic("implement me")
}

func (self *testLink) SendAcknowledgement(acknowledgement *xgress.Acknowledgement) error {
	panic("implement me")
}

func (self *testLink) SendControl(control *xgress.Control) error {
	panic("implement me")
}

func (self *testLink) InspectCircuit(circuitDetail *xgress.CircuitInspectDetail) {
	panic("implement me")
}

func (self *testLink) Key() string {
	return self.key
}

func (self *testLink) Init(metricsRegistry metrics.Registry) error {
	panic("implement me")
}

func (self *testLink) Close() error {
	panic("implement me")
}

func (self *testLink) CloseNotified() error {
	panic("implement me")
}

func (self *testLink) DestinationId() string {
	return self.destId
}

func (self *testLink) DestVersion() string {
	panic("implement me")
}

func (self *testLink) LinkProtocol() string {
	return "tls"
}

func (self *testLink) DialAddress() string {
	panic("implement me")
}

func (self *testLink) CloseOnce(f func()) {
	panic("implement me")
}

func (self *testLink) IsClosed() bool {
	panic("implement me")
}

func (self *testLink) InspectLink() *inspect.LinkInspectDetail {
	panic("implement me")
}

func (self *testLink) GetLinkConnState() *ctrl_pb.LinkConnState {
	panic("implement me")
}

func (self *testLink) IsDialed() bool {
	panic("implement me")
}

func (self *testLink) Iteration() uint32 {
	panic("implement me")
}

func (self *testLink) AreFaultsSent() bool {
	panic("implement me")
}

func (self *testLink) DuplicatesRejected() uint32 {
	panic("implement me")
}

func newTestLink(reg *linkRegistryImpl) *testLink {
	linkId := idgen.MustNewUUIDString()
	destId := idgen.MustNewUUIDString()
	linkKey := reg.GetLinkKey("", "tls", destId, "")
	return &testLink{
		id:     linkId,
		key:    linkKey,
		destId: destId,
	}
}

func newTestEnv() *testEnv {
	closeNotify := make(chan struct{})

	registryConfig := metrics.DefaultUsageRegistryConfig("test", closeNotify)
	metricsRegistry := metrics.NewUsageRegistry(registryConfig)

	testEnv := &testEnv{
		metricsRegistry: metricsRegistry,
		closeNotify:     closeNotify,
		config:          &env.Config{},
	}

	testEnv.config.Ctrl.DefaultRequestTimeout = time.Second
	testEnv.ctrls = env.NewNetworkControllers(testEnv, env.NewDefaultHeartbeatOptions())
	return testEnv
}

func Test_gcLinkMetrics(t *testing.T) {
	routerEnv := newTestEnv()
	defer close(routerEnv.closeNotify)

	reg := NewLinkRegistry(routerEnv).(*linkRegistryImpl)
	orphaned := reg.getOrphanedLinkMetrics()

	req := require.New(t)
	req.Equal(0, len(orphaned))

	linkId := idgen.MustNewUUIDString()
	registry := routerEnv.metricsRegistry

	linkMetric := func(linkId, name string) string {
		return "link." + linkId + "." + name
	}

	addLinkMetrics := func(linkId string) map[string]metrics.Metric {
		result := map[string]metrics.Metric{}
		addHist := func(name string) {
			result[linkMetric(linkId, name)] = registry.Histogram(linkMetric(linkId, name))
		}

		addMeter := func(name string) {
			result[linkMetric(linkId, name)] = registry.Meter(linkMetric(linkId, name))
		}

		addHist("latency")
		addHist("queue_time")
		addMeter("tx.bytesrate")
		addMeter("tx.msgrate")
		addHist("tx.msgsize")
		addMeter("rx.bytesrate")
		addMeter("rx.msgrate")
		addHist("rx.msgsize")

		metricId := "link.dropped_msgs:" + linkId
		result[metricId] = registry.Meter(metricId)

		return result
	}

	checkLinkMetrics := func(linkId string, m map[string]metrics.Metric, contains bool) {
		checkMetric := func(name string) {
			metricId := linkMetric(linkId, name)
			if contains {
				req.True(m[metricId] != nil, "missing metric %s", metricId)
			} else {
				req.False(m[metricId] != nil, "should not have metric %s", metricId)
			}
		}
		checkPostFixMetric := func(name string) {
			metricId := "link." + name + ":" + linkId
			if contains {
				req.True(m[metricId] != nil, "missing metric %s", metricId)
			} else {
				req.False(m[metricId] != nil, "should not have metric %s", metricId)
			}
		}
		checkMetric("latency")
		checkMetric("queue_time")
		checkMetric("tx.bytesrate")
		checkMetric("tx.msgrate")
		checkMetric("tx.msgsize")
		checkMetric("rx.bytesrate")
		checkMetric("rx.msgrate")
		checkMetric("rx.msgsize")
		checkPostFixMetric("dropped_msgs")
	}

	checkLinkMetricsContains := func(linkId string, m map[string]metrics.Metric) {
		checkLinkMetrics(linkId, m, true)
	}

	checkLinkMetricsDoesntHave := func(linkId string, m map[string]metrics.Metric) {
		checkLinkMetrics(linkId, m, false)
	}

	getRegistryMetrics := func() map[string]metrics.Metric {
		result := map[string]metrics.Metric{}
		registry.EachMetric(func(name string, metric metrics.Metric) {
			result[name] = metric
		})
		return result
	}

	l := addLinkMetrics(linkId)
	registry.Histogram("unrelated.to.links.hist")
	registry.Meter("unrelated.to.links.meter")

	orphaned = reg.getOrphanedLinkMetrics()
	req.Equal(len(l), len(orphaned))
	checkLinkMetricsContains(linkId, orphaned)

	orphaned = reg.gcLinkMetrics(nil)
	req.Equal(len(l), len(orphaned))
	checkLinkMetricsContains(linkId, orphaned)

	orphaned = reg.gcLinkMetrics(orphaned)
	req.Equal(0, len(orphaned))
	checkLinkMetricsDoesntHave(linkId, getRegistryMetrics())

	req.Equal(2, len(getRegistryMetrics()))

	linkId2 := idgen.MustNewUUIDString()
	link3 := newTestLink(reg)
	link4 := newTestLink(reg)
	linkId5 := idgen.MustNewUUIDString()

	reg.linkByIdMap[link3.id] = link3
	reg.linkMap[link3.Key()] = link4

	dest := newLinkDest(link4.DestinationId())
	reg.destinations[link4.DestinationId()] = dest
	dest.linkMap[link4.key] = &linkState{
		linkKey: link4.key,
		linkId:  link4.id,
		status:  StatusPending,
		dest:    dest,
	}

	addLinkMetrics(linkId2)
	addLinkMetrics(link3.id)
	addLinkMetrics(link4.id)
	addLinkMetrics(linkId5)

	req.Equal(9*4+2, len(getRegistryMetrics()))

	orphaned = reg.gcLinkMetrics(nil)
	req.Equal(18, len(orphaned))
	checkLinkMetricsContains(linkId2, orphaned)
	checkLinkMetricsContains(linkId5, orphaned)
	checkLinkMetricsDoesntHave(link3.id, orphaned)
	checkLinkMetricsDoesntHave(link4.id, orphaned)

	req.Equal(9*4+2, len(getRegistryMetrics()))

	orphaned = reg.gcLinkMetrics(orphaned)
	req.Equal(0, len(orphaned))
	req.Equal(9*2+2, len(getRegistryMetrics()))

	checkLinkMetricsContains(link3.id, getRegistryMetrics())
	checkLinkMetricsContains(link4.id, getRegistryMetrics())
	checkLinkMetricsDoesntHave(linkId2, getRegistryMetrics())
	checkLinkMetricsDoesntHave(linkId5, getRegistryMetrics())
}
