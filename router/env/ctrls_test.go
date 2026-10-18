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

package env

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openziti/channel/v4"
	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/transport/v2"
	"github.com/openziti/transport/v2/tls"
	"github.com/stretchr/testify/require"
)

// Test_networkControllers_Dialing locks in that the router keeps one dial per controller endpoint, stops
// dialing an endpoint that reaches a controller it is already connected to, and doesn't register a
// controller reached on an endpoint removed while it was dialed.
func Test_networkControllers_Dialing(t *testing.T) {
	t.Run("an endpoint added back while it is dialed is not dialed again", func(t *testing.T) {
		req := require.New(t)

		// arrange: ctrl1's dial is under way
		ctrls, dialer := newTestNetworkControllers(t)
		ep1, ep2 := testCtrlEndpoint("ctrl1"), testCtrlEndpoint("ctrl2")
		dialer.ctrlIds[ep1] = "ctrl1"
		dialer.ctrlIds[ep2] = "ctrl2"
		ep1Dial := dialer.gate(ep1)
		ctrls.UpdateControllerEndpoints([]string{ep1, ep2})
		select {
		case <-ep1Dial.started:
		case <-time.After(5 * time.Second):
			req.FailNow("the first dial did not start")
		}

		// act
		ctrls.UpdateControllerEndpoints([]string{ep2})
		ctrls.UpdateControllerEndpoints([]string{ep1, ep2})
		ep1Dial.release()

		// assert
		requireRegisteredAt(t, ctrls, "ctrl1", ep1)
		requireRegisteredAt(t, ctrls, "ctrl2", ep2)
		req.Eventually(func() bool { return !ctrls.isDialing(ep1) }, 5*time.Second, time.Millisecond)
		req.Never(func() bool { return dialer.dialCount(ep1) > 1 }, 200*time.Millisecond, 10*time.Millisecond,
			"an endpoint added back during its dial must not be dialed again")
	})

	t.Run("an endpoint reaching an already connected controller stops dialing", func(t *testing.T) {
		req := require.New(t)

		// arrange: ctrl1 is connected by name, and its address reaches it too
		ctrls, dialer := newTestNetworkControllers(t)
		viaName, viaAddress := testCtrlEndpoint("ctrl1.example.com"), testCtrlEndpoint("10.0.0.1")
		dialer.ctrlIds[viaName] = "ctrl1"
		dialer.ctrlIds[viaAddress] = "ctrl1"
		ctrls.UpdateControllerEndpoints([]string{viaName})
		requireRegisteredAt(t, ctrls, "ctrl1", viaName)

		// act
		ctrls.UpdateControllerEndpoints([]string{viaName, viaAddress})

		// assert
		req.Eventually(func() bool { return !ctrls.isDialing(viaAddress) }, 5*time.Second, time.Millisecond,
			"a dial reaching an already connected controller must stop")
		req.Never(func() bool { return dialer.dialCount(viaAddress) > 1 }, 300*time.Millisecond, 10*time.Millisecond,
			"a dial reaching an already connected controller must not retry")
		requireRegisteredAt(t, ctrls, "ctrl1", viaName)

		t.Run("it is dialed again once that controller's endpoint is removed", func(t *testing.T) {
			req := require.New(t)

			// act
			ctrls.UpdateControllerEndpoints([]string{viaAddress})

			// assert
			requireRegisteredAt(t, ctrls, "ctrl1", viaAddress)
			req.Equal(2, dialer.dialCount(viaAddress))
		})
	})

	t.Run("a controller reached on an endpoint removed during its dial is not registered", func(t *testing.T) {
		req := require.New(t)

		// arrange: ctrl1's dial is under way
		ctrls, dialer := newTestNetworkControllers(t)
		ep1, ep2 := testCtrlEndpoint("ctrl1"), testCtrlEndpoint("ctrl2")
		dialer.ctrlIds[ep1] = "ctrl1"
		dialer.ctrlIds[ep2] = "ctrl2"
		ep1Dial := dialer.gate(ep1)
		ctrls.UpdateControllerEndpoints([]string{ep1})
		select {
		case <-ep1Dial.started:
		case <-time.After(5 * time.Second):
			req.FailNow("the first dial did not start")
		}

		// act
		ctrls.UpdateControllerEndpoints([]string{ep2})
		ep1Dial.release()

		// assert
		requireRegisteredAt(t, ctrls, "ctrl2", ep2)
		req.Eventually(func() bool { return !ctrls.isDialing(ep1) }, 5*time.Second, time.Millisecond)
		req.Nil(ctrls.GetNetworkController("ctrl1"), "a controller reached on a removed endpoint must not be registered")
		req.Equal([]string{ep2}, ctrls.GetEndpoints())
	})
}

// requireRegisteredAt fails the test unless ctrlId is registered within five seconds, at endpoint, on an
// open channel.
func requireRegisteredAt(t *testing.T, ctrls *networkControllers, ctrlId string, endpoint string) {
	t.Helper()
	require.Eventually(t, func() bool {
		ctrl := ctrls.GetNetworkController(ctrlId)
		return ctrl != nil && ctrl.Address() == endpoint && !ctrl.Channel().IsClosed()
	}, 5*time.Second, time.Millisecond, "%s was not registered at %s", ctrlId, endpoint)
}

// newTestNetworkControllers returns a controller set that dials through the returned dialer.
func newTestNetworkControllers(t *testing.T) (*networkControllers, *testCtrlDialer) {
	transport.AddAddressParser(tls.AddressParser{})
	dialer := newTestCtrlDialer(t)
	ctrls := NewNetworkControllers(time.Minute, dialer.dial, NewDefaultHeartbeatOptions()).(*networkControllers)
	return ctrls, dialer
}

func testCtrlEndpoint(name string) string {
	return "tls:" + name + ":6262"
}

func (self *networkControllers) isDialing(endpoint string) bool {
	self.lock.Lock()
	defer self.lock.Unlock()
	_, ok := self.dialing[endpoint]
	return ok
}

// testCtrlDialer reaches the controller each endpoint names, counting dials. A gated endpoint's first
// dial waits until its gate is released. Bind errors are wrapped without keeping them in the chain.
type testCtrlDialer struct {
	t       *testing.T
	lock    sync.Mutex
	ctrlIds map[string]string
	dials   map[string]int
	gates   map[string]*dialGate
}

func newTestCtrlDialer(t *testing.T) *testCtrlDialer {
	return &testCtrlDialer{
		t:       t,
		ctrlIds: map[string]string{},
		dials:   map[string]int{},
		gates:   map[string]*dialGate{},
	}
}

// gate makes endpoint's next dial wait until the returned gate is released. The gate is released when the
// test ends.
func (d *testCtrlDialer) gate(endpoint string) *dialGate {
	d.lock.Lock()
	defer d.lock.Unlock()
	g := &dialGate{started: make(chan struct{}), released: make(chan struct{})}
	d.gates[endpoint] = g
	d.t.Cleanup(g.release)
	return g
}

func (d *testCtrlDialer) dialCount(endpoint string) int {
	d.lock.Lock()
	defer d.lock.Unlock()
	return d.dials[endpoint]
}

func (d *testCtrlDialer) dial(address transport.Address, bindHandler channel.BindHandler) error {
	endpoint := address.String()

	d.lock.Lock()
	d.dials[endpoint]++
	ctrlId := d.ctrlIds[endpoint]
	g := d.gates[endpoint]
	delete(d.gates, endpoint)
	d.lock.Unlock()

	if g != nil {
		close(g.started)
		<-g.released
	}

	version, err := versions.StdVersionEncDec.Encode(&versions.VersionInfo{Version: "v1.6.0"})
	if err != nil {
		return err
	}
	ch := &testCtrlChannel{
		id:       ctrlId,
		underlay: &testCtrlUnderlay{headers: map[int32][]byte{channel.HelloVersionHeader: version}},
	}

	if err = bindHandler.BindChannel(&testCtrlBinding{ch: ch}); err != nil {
		_ = ch.Close()
		return fmt.Errorf("error connecting ctrl (%v)", err)
	}
	return nil
}

// dialGate holds one dial until released. started is closed when the dial reaches the gate.
type dialGate struct {
	started     chan struct{}
	released    chan struct{}
	releaseOnce sync.Once
}

// release lets the held dial continue. It is safe to call more than once.
func (g *dialGate) release() {
	g.releaseOnce.Do(func() { close(g.released) })
}

// testCtrlChannel is a controller's channel, identified by the controller's id.
type testCtrlChannel struct {
	channel.Channel
	id       string
	closed   atomic.Bool
	underlay *testCtrlUnderlay
}

func (c *testCtrlChannel) Id() string {
	return c.id
}

func (c *testCtrlChannel) Label() string {
	return c.id
}

func (c *testCtrlChannel) IsClosed() bool {
	return c.closed.Load()
}

func (c *testCtrlChannel) Close() error {
	c.closed.Store(true)
	return nil
}

func (c *testCtrlChannel) Underlay() channel.Underlay {
	return c.underlay
}

// testCtrlUnderlay carries the hello headers a controller sends.
type testCtrlUnderlay struct {
	channel.Underlay
	headers map[int32][]byte
}

func (u *testCtrlUnderlay) Headers() map[int32][]byte {
	return u.headers
}

// testCtrlBinding binds a testCtrlChannel, ignoring receive handlers.
type testCtrlBinding struct {
	channel.Binding
	ch channel.Channel
}

func (b *testCtrlBinding) GetChannel() channel.Channel {
	return b.ch
}

func (b *testCtrlBinding) AddReceiveHandlerF(int32, channel.ReceiveHandlerF) {}
