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

type testCtrlUnderlay struct {
	channel.Underlay
	headers map[int32][]byte
}

func (u *testCtrlUnderlay) Headers() map[int32][]byte {
	return u.headers
}

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

type testCtrlBinding struct {
	channel.Binding
	ch channel.Channel
}

func (b *testCtrlBinding) GetChannel() channel.Channel {
	return b.ch
}

func (b *testCtrlBinding) AddReceiveHandlerF(int32, channel.ReceiveHandlerF) {}

// testCtrlDialer reaches the controller each endpoint names, counting dials. A gated endpoint's first
// dial waits until the gate is released. Bind errors are wrapped without keeping them in the chain.
type testCtrlDialer struct {
	t       *testing.T
	lock    sync.Mutex
	ctrlIds map[string]string
	dials   map[string]int
	gates   map[string]chan struct{}
	started map[string]chan struct{}
}

func newTestCtrlDialer(t *testing.T) *testCtrlDialer {
	return &testCtrlDialer{
		t:       t,
		ctrlIds: map[string]string{},
		dials:   map[string]int{},
		gates:   map[string]chan struct{}{},
		started: map[string]chan struct{}{},
	}
}

// gate makes endpoint's first dial wait for release, and returns a channel closed once it starts.
func (d *testCtrlDialer) gate(endpoint string) (started <-chan struct{}, release func()) {
	d.lock.Lock()
	defer d.lock.Unlock()
	gateC := make(chan struct{})
	startedC := make(chan struct{})
	d.gates[endpoint] = gateC
	d.started[endpoint] = startedC
	var once sync.Once
	release = func() {
		once.Do(func() {
			close(gateC)
		})
	}
	d.t.Cleanup(release)
	return startedC, release
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
	gateC := d.gates[endpoint]
	startedC := d.started[endpoint]
	delete(d.gates, endpoint)
	d.lock.Unlock()

	if gateC != nil {
		close(startedC)
		<-gateC
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

func requireRegisteredAt(t *testing.T, ctrls *networkControllers, ctrlId string, endpoint string) {
	require.Eventually(t, func() bool {
		ctrl := ctrls.GetNetworkController(ctrlId)
		return ctrl != nil && ctrl.Address() == endpoint && !ctrl.Channel().IsClosed()
	}, 5*time.Second, time.Millisecond, "%s was not registered at %s", ctrlId, endpoint)
}

// Test_NetworkControllers_ReaddedEndpointIsNotDialedTwice: an endpoint removed and added again while its
// dial is under way keeps that dial, rather than starting a second one that would reach the same
// controller.
func Test_NetworkControllers_ReaddedEndpointIsNotDialedTwice(t *testing.T) {
	req := require.New(t)
	ctrls, dialer := newTestNetworkControllers(t)
	ep1, ep2 := testCtrlEndpoint("ctrl1"), testCtrlEndpoint("ctrl2")
	dialer.ctrlIds[ep1] = "ctrl1"
	dialer.ctrlIds[ep2] = "ctrl2"
	started, release := dialer.gate(ep1)

	ctrls.UpdateControllerEndpoints([]string{ep1, ep2})
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		req.FailNow("the first dial did not start")
	}

	ctrls.UpdateControllerEndpoints([]string{ep2})
	ctrls.UpdateControllerEndpoints([]string{ep1, ep2})
	release()

	requireRegisteredAt(t, ctrls, "ctrl1", ep1)
	requireRegisteredAt(t, ctrls, "ctrl2", ep2)
	req.Eventually(func() bool { return !ctrls.isDialing(ep1) }, 5*time.Second, time.Millisecond)
	time.Sleep(200 * time.Millisecond)
	req.Equal(1, dialer.dialCount(ep1), "an endpoint added back during its dial must not be dialed again")
}

// Test_NetworkControllers_DuplicateEndpointStopsAndIsDialedWhenItsControllerGoes: an endpoint reaching a
// controller already connected on another endpoint stops dialing, and is dialed again once the endpoint
// that controller was connected on is removed.
func Test_NetworkControllers_DuplicateEndpointStopsAndIsDialedWhenItsControllerGoes(t *testing.T) {
	req := require.New(t)
	ctrls, dialer := newTestNetworkControllers(t)
	viaName, viaAddress := testCtrlEndpoint("ctrl1.example.com"), testCtrlEndpoint("10.0.0.1")
	dialer.ctrlIds[viaName] = "ctrl1"
	dialer.ctrlIds[viaAddress] = "ctrl1"

	ctrls.UpdateControllerEndpoints([]string{viaName})
	requireRegisteredAt(t, ctrls, "ctrl1", viaName)

	ctrls.UpdateControllerEndpoints([]string{viaName, viaAddress})
	req.Eventually(func() bool { return !ctrls.isDialing(viaAddress) }, 5*time.Second, time.Millisecond,
		"a dial reaching an already connected controller must stop")
	time.Sleep(300 * time.Millisecond)
	req.Equal(1, dialer.dialCount(viaAddress), "a dial reaching an already connected controller must not retry")
	requireRegisteredAt(t, ctrls, "ctrl1", viaName)

	ctrls.UpdateControllerEndpoints([]string{viaAddress})
	requireRegisteredAt(t, ctrls, "ctrl1", viaAddress)
	req.Equal(2, dialer.dialCount(viaAddress))
}

// Test_NetworkControllers_RemovedEndpointIsNotRegistered: a dial that completes after its endpoint was
// removed doesn't register the controller it reached.
func Test_NetworkControllers_RemovedEndpointIsNotRegistered(t *testing.T) {
	req := require.New(t)
	ctrls, dialer := newTestNetworkControllers(t)
	ep1, ep2 := testCtrlEndpoint("ctrl1"), testCtrlEndpoint("ctrl2")
	dialer.ctrlIds[ep1] = "ctrl1"
	dialer.ctrlIds[ep2] = "ctrl2"
	started, release := dialer.gate(ep1)

	ctrls.UpdateControllerEndpoints([]string{ep1})
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		req.FailNow("the first dial did not start")
	}

	ctrls.UpdateControllerEndpoints([]string{ep2})
	release()

	requireRegisteredAt(t, ctrls, "ctrl2", ep2)
	req.Eventually(func() bool { return !ctrls.isDialing(ep1) }, 5*time.Second, time.Millisecond)
	req.Nil(ctrls.GetNetworkController("ctrl1"), "a controller reached on a removed endpoint must not be registered")
	req.Equal([]string{ep2}, ctrls.GetEndpoints())
}
