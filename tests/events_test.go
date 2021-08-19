// +build apitests

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

package tests

import (
	"fmt"
	events2 "github.com/openziti/edge/events"
	"github.com/openziti/fabric/controller/xt_smartrouting"
	"github.com/openziti/fabric/events"
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/openziti/foundation/util/stringz"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"reflect"
	"sync"
	"testing"
	"time"
)

type eventsCollector struct {
	sync.Mutex
	events           []interface{}
	usageEventNotify chan struct{}
	notified         concurrenz.AtomicBoolean
}

func (self *eventsCollector) waitForUsage(timeout time.Duration) error {
	select {
	case <-self.usageEventNotify:
		return nil
	case <-time.After(timeout):
		return errors.New("timed out waiting for usage data")
	}
}

func (self *eventsCollector) acceptEvent(event interface{}) {
	self.Lock()
	defer self.Unlock()
	self.events = append(self.events, event)
	logrus.Warnf("%v: %v %+v\n", reflect.TypeOf(event), event, event)
}

func (self *eventsCollector) AcceptUsageEvent(event *events.UsageEvent) {
	self.acceptEvent(event)
	if self.notified.CompareAndSwap(false, true) {
		close(self.usageEventNotify)
	}
}

func (self *eventsCollector) AcceptSessionEvent(event *events2.SessionEvent) {
	self.acceptEvent(event)
}

func (self *eventsCollector) AcceptCircuitEvent(event *events.CircuitEvent) {
	self.acceptEvent(event)
}

func (self *eventsCollector) PopNextEvent(ctx *TestContext) interface{} {
	ctx.Req.True(len(self.events) > 0)
	result := self.events[0]
	self.events = self.events[1:]
	return result
}

func Test_EventsTest(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()

	ec := &eventsCollector{
		usageEventNotify: make(chan struct{}),
	}

	unregisterFabricSessionEventsHandler := events.RegisterCircuitEventHandler(ec)
	defer unregisterFabricSessionEventsHandler()

	events2.AddSessionEventHandler(ec)
	defer events2.RemoveSessionEventHandler(ec)

	unregisterUsageEventsHandler := events.RegisterUsageEventHandler(ec)
	defer unregisterUsageEventsHandler()

	ctx.StartServer()

	ctx.RequireAdminManagementApiLogin()
	ctx.RequireAdminClientApiLogin()

	ctx.CreateEnrollAndStartEdgeRouter()

	service := ctx.AdminManagementSession.RequireNewServiceAccessibleToAll(xt_smartrouting.Name)

	ctx.CreateEnrollAndStartEdgeRouter()

	hostIdentity, hostContext := ctx.AdminManagementSession.RequireCreateSdkContext()
	defer hostContext.Close()

	listener, err := hostContext.Listen(service.Name)
	ctx.Req.NoError(err)
	defer func() { _ = listener.Close() }()

	testServer := newTestServer(listener, func(conn *testServerConn) error {
		conn.ReadString(128, time.Second)
		return conn.server.close()
	})
	testServer.start()

	clientIdentity, clientContext := ctx.AdminManagementSession.RequireCreateSdkContext()
	defer clientContext.Close()

	conn := ctx.WrapConn(clientContext.Dial(service.Name))
	defer func() { _ = conn.Close() }()

	conn.WriteString("hello, hello, how are you?", time.Second)

	testServer.waitForDone(ctx, 5*time.Second)
	ctx.router.MetricsRegistry().Flush()
	err = ec.waitForUsage(2 * time.Minute)
	ctx.Req.NoError(err)

	ctx.Teardown()

	for _, event := range ec.events {
		fmt.Printf("%v: %v %+v\n", reflect.TypeOf(event), event, event)
	}

	event := ec.PopNextEvent(ctx)
	edgeSession, ok := event.(*events2.SessionEvent)
	ctx.Req.True(ok)
	ctx.Req.Equal("edge.sessions", edgeSession.Namespace)
	ctx.Req.Equal("created", edgeSession.EventType)
	ctx.Req.Equal(hostIdentity.Id, edgeSession.IdentityId)

	event = ec.PopNextEvent(ctx)
	edgeSession, ok = event.(*events2.SessionEvent)
	ctx.Req.True(ok)
	ctx.Req.Equal("edge.sessions", edgeSession.Namespace)
	ctx.Req.Equal("created", edgeSession.EventType)
	ctx.Req.Equal(clientIdentity.Id, edgeSession.IdentityId)

	event = ec.PopNextEvent(ctx)
	circuitEvent, ok := event.(*events.CircuitEvent)
	ctx.Req.True(ok)
	ctx.Req.Equal("fabric.circuits", circuitEvent.Namespace)
	ctx.Req.Equal("created", circuitEvent.EventType)
	ctx.Req.Equal(service.Id, circuitEvent.ServiceId)
	ctx.Req.Equal(edgeSession.Id, circuitEvent.ClientId)

	for i := 0; i < 3; i++ {
		event = ec.PopNextEvent(ctx)
		if usage, ok := event.(*events.UsageEvent); ok {
			ctx.Req.Equal("fabric.usage", usage.Namespace)
			ctx.Req.Equal(uint32(2), usage.Version)
			ctx.Req.Equal(circuitEvent.CircuitId, usage.CircuitId)
			expected := []string{"usage.ingress.rx", "usage.egress.tx"}
			ctx.Req.True(stringz.Contains(expected, usage.EventType), "was %v, expected one of %+v", usage.EventType, expected)
			ctx.Req.Equal(ctx.edgeRouterEntity.id, usage.SourceId)
			ctx.Req.Equal(uint64(26), usage.Usage)
		} else if fabricSession, ok := event.(*events.CircuitEvent); ok {
			ctx.Req.Equal("fabric.circuits", fabricSession.Namespace)
			ctx.Req.Equal("deleted", fabricSession.EventType)
			ctx.Req.Equal(edgeSession.Id, fabricSession.ClientId)
		} else {
			ctx.Req.Fail("unexpected event type: %v", reflect.TypeOf(event))
		}
	}
}
