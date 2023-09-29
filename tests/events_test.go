//go:build apitests

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

package tests

import (
	"fmt"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/controller/event"
	"github.com/openziti/ziti/controller/xt_smartrouting"
	"reflect"
	"sync"
	"testing"
	"time"
)

type eventsCollector struct {
	sync.Mutex
	events chan interface{}
}

func (self *eventsCollector) acceptEvent(event interface{}) {
	self.events <- event
	fmt.Printf("\nNEXT EVENT: %v: %v %+v\n", reflect.TypeOf(event), event, event)
}

func (self *eventsCollector) AcceptUsageEvent(event *event.UsageEvent) {
	self.acceptEvent(event)
}

func (self *eventsCollector) AcceptSessionEvent(event *event.SessionEvent) {
	self.acceptEvent(event)
}

func (self *eventsCollector) AcceptCircuitEvent(event *event.CircuitEvent) {
	self.acceptEvent(event)
}

func (self *eventsCollector) PopNextEvent(ctx *TestContext, desc string, timeout time.Duration) interface{} {
	select {
	case evt := <-self.events:
		return evt
	case <-time.After(timeout):
		ctx.Fail("timed out waiting for event", desc)
		return nil
	}
}

func Test_EventsTest(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()

	ctx.StartServer()

	ec := &eventsCollector{
		events: make(chan interface{}, 50),
	}

	dispatcher := ctx.fabricController.GetEventDispatcher()

	dispatcher.AddCircuitEventHandler(ec)
	defer dispatcher.RemoveCircuitEventHandler(ec)

	dispatcher.AddSessionEventHandler(ec)
	defer dispatcher.RemoveSessionEventHandler(ec)

	dispatcher.AddUsageEventHandler(ec)
	defer dispatcher.RemoveUsageEventHandler(ec)

	ctx.RequireAdminManagementApiLogin()
	ctx.RequireAdminClientApiLogin()

	ctx.CreateEnrollAndStartEdgeRouter()

	service := ctx.AdminManagementSession.RequireNewServiceAccessibleToAll(xt_smartrouting.Name)

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
	// TODO: Figure out how to make this test faster. Was using ctx.router.GetMetricsRegistry().Flush(), but it's not ideal
	ctx.Req.NoError(err)

	evt := ec.PopNextEvent(ctx, "edge.sessions.created", time.Second)
	edgeSession, ok := evt.(*event.SessionEvent)
	ctx.Req.True(ok)
	ctx.Req.Equal("edge.sessions", edgeSession.Namespace)
	ctx.Req.Equal("created", edgeSession.EventType)
	ctx.Req.Equal(hostIdentity.Id, edgeSession.IdentityId)

	evt = ec.PopNextEvent(ctx, "edge.sessions.created", time.Second)
	edgeSession, ok = evt.(*event.SessionEvent)
	ctx.Req.True(ok)
	ctx.Req.Equal("edge.sessions", edgeSession.Namespace)
	ctx.Req.Equal("created", edgeSession.EventType)
	ctx.Req.Equal(clientIdentity.Id, edgeSession.IdentityId)

	evt = ec.PopNextEvent(ctx, "fabric.circuits.created", time.Second)
	circuitEvent, ok := evt.(*event.CircuitEvent)
	ctx.Req.True(ok)
	ctx.Req.Equal("fabric.circuits", circuitEvent.Namespace)
	ctx.Req.Equal("created", string(circuitEvent.EventType))
	ctx.Req.Equal(service.Id, circuitEvent.ServiceId)
	ctx.Req.Equal(edgeSession.Id, circuitEvent.ClientId)

	timeout := time.Minute * 2
	for i := 0; i < 3; i++ {
		evt = ec.PopNextEvent(ctx, fmt.Sprintf("usage or circuits deleted %v", i+1), timeout)
		if usage, ok := evt.(*event.UsageEvent); ok {
			timeout = time.Second * 10
			ctx.Req.Equal("fabric.usage", usage.Namespace)
			ctx.Req.Equal(uint32(2), usage.Version)
			ctx.Req.Equal(circuitEvent.CircuitId, usage.CircuitId)
			expected := []string{"usage.ingress.rx", "usage.egress.tx"}
			ctx.Req.True(stringz.Contains(expected, usage.EventType), "was %v, expected one of %+v", usage.EventType, expected)
			ctx.Req.Equal(ctx.edgeRouterEntity.id, usage.SourceId)
			ctx.Req.Equal(uint64(26), usage.Usage)
		} else if circuitEvent, ok := evt.(*event.CircuitEvent); ok {
			ctx.Req.Equal("fabric.circuits", circuitEvent.Namespace)
			ctx.Req.Equal("deleted", string(circuitEvent.EventType))
			ctx.Req.Equal(edgeSession.Id, circuitEvent.ClientId)
		} else {
			ctx.Req.Fail("unexpected event type: %v", reflect.TypeOf(evt))
		}
	}
}
