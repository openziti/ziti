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

package router

import (
	"testing"
	"time"

	"github.com/openziti/channel/v5"
	"github.com/openziti/ziti/v2/common/servermetrics"
	"github.com/openziti/ziti/v2/common/servermetrics/metrics_pb"
	"github.com/openziti/ziti/v2/router/env"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// testCtrls serves a fixed controller set. Other env.NetworkControllers methods are unimplemented.
type testCtrls struct {
	env.NetworkControllers
	ctrls map[string]env.NetworkController
}

func (self *testCtrls) GetAll() map[string]env.NetworkController {
	return self.ctrls
}

func (self *testCtrls) DefaultRequestTimeout() time.Duration {
	return time.Second
}

type testCtrl struct {
	env.NetworkController
	ch channel.Channel
}

func (self *testCtrl) Channel() channel.Channel {
	return self.ch
}

// metricsSink records the metrics messages written to it, or fails every send with err when set.
type metricsSink struct {
	channel.Channel
	err         error
	received    []*metrics_pb.MetricsMessage
	closeNotify chan struct{}
}

func newMetricsSink() *metricsSink {
	return &metricsSink{closeNotify: make(chan struct{})}
}

func (self *metricsSink) Send(s channel.Sendable) error {
	if self.err != nil {
		return self.err
	}
	msg := &metrics_pb.MetricsMessage{}
	if err := proto.Unmarshal(s.Msg().Body, msg); err != nil {
		return err
	}
	self.received = append(self.received, msg)
	s.SendListener().NotifyAfterWrite()
	return nil
}

func (self *metricsSink) CloseNotify() <-chan struct{} {
	return self.closeNotify
}

func (self *metricsSink) eventIds() []string {
	var result []string
	for _, msg := range self.received {
		result = append(result, msg.EventId)
	}
	return result
}

func acceptWithin(t *testing.T, reporter servermetrics.Handler, eventId string) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		reporter.AcceptMetrics(&metrics_pb.MetricsMessage{EventId: eventId})
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("AcceptMetrics(%s) did not return", eventId)
	}
}

func TestControllersReporterQueuesWhileNoControllerIsRegistered(t *testing.T) {
	ctrls := &testCtrls{}
	reporter := NewControllersReporter(ctrls, 10)

	acceptWithin(t, reporter, "m1")
	acceptWithin(t, reporter, "m2")

	sink := newMetricsSink()
	ctrls.ctrls = map[string]env.NetworkController{"ctrl1": &testCtrl{ch: sink}}

	acceptWithin(t, reporter, "m3")
	require.Equal(t, []string{"m1", "m2", "m3"}, sink.eventIds())
}

func TestControllersReporterQueuesWhileEverySendFails(t *testing.T) {
	sink := newMetricsSink()
	sink.err = channel.ClosedError{}
	reporter := NewControllersReporter(&testCtrls{
		ctrls: map[string]env.NetworkController{"ctrl1": &testCtrl{ch: sink}},
	}, 10)

	acceptWithin(t, reporter, "m1")
	acceptWithin(t, reporter, "m2")
	require.Empty(t, sink.received)

	sink.err = nil
	acceptWithin(t, reporter, "m3")
	require.Equal(t, []string{"m1", "m2", "m3"}, sink.eventIds())
}

func TestControllersReporterDropsOldestBeyondBacklog(t *testing.T) {
	ctrls := &testCtrls{}
	reporter := NewControllersReporter(ctrls, 2)

	for _, id := range []string{"m1", "m2", "m3", "m4"} {
		acceptWithin(t, reporter, id)
	}

	sink := newMetricsSink()
	ctrls.ctrls = map[string]env.NetworkController{"ctrl1": &testCtrl{ch: sink}}

	acceptWithin(t, reporter, "m5")
	require.Equal(t, []string{"m4", "m5"}, sink.eventIds())
}

func TestControllersReporterMarksCopiesAfterFirstDeliveryDoNotPropagate(t *testing.T) {
	failing := newMetricsSink()
	failing.err = channel.ClosedError{}
	sink1 := newMetricsSink()
	sink2 := newMetricsSink()
	reporter := NewControllersReporter(&testCtrls{
		ctrls: map[string]env.NetworkController{
			"failing": &testCtrl{ch: failing},
			"ctrl1":   &testCtrl{ch: sink1},
			"ctrl2":   &testCtrl{ch: sink2},
		},
	}, 10)

	acceptWithin(t, reporter, "m1")

	require.Len(t, sink1.received, 1)
	require.Len(t, sink2.received, 1)
	require.NotEqual(t, sink1.received[0].DoNotPropagate, sink2.received[0].DoNotPropagate,
		"exactly one delivered copy should be propagated")
}
