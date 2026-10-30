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

package handler_xgress

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openziti/channel/v4"
	"github.com/openziti/sdk-golang/xgress"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/router/env"
	"github.com/openziti/ziti/v2/router/forwarder"
	"github.com/stretchr/testify/require"
)

const (
	testCircuitId = "circuit"
	testSrcAddr   = "src"
	testDstAddr   = "dst"
)

// busyDestination is a next hop whose sends time out, as a congested link's do, until it has refused
// timeouts of them; later sends are accepted.
type busyDestination struct {
	env.Destination
	timeouts   int32
	timeoutErr error
	attempts   atomic.Int32
}

func (self *busyDestination) SendPayload(*xgress.Payload, time.Duration, xgress.PayloadType) error {
	if self.attempts.Add(1) <= self.timeouts {
		return self.timeoutErr
	}
	return nil
}

func (self *busyDestination) Unrouted() {}

func (self *busyDestination) GetTimeOfLastRxFromLink() int64 {
	return 0
}

// unwrittenSender queues every message and never writes any of them to the wire.
type unwrittenSender struct{}

func (unwrittenSender) Send(channel.Sendable) error {
	return nil
}

func (unwrittenSender) TrySend(channel.Sendable) (bool, error) {
	return true, nil
}

func (unwrittenSender) CloseNotify() <-chan struct{} {
	return nil
}

// newTimeoutError returns a channel timeout error. The channel package exports no constructor, so it
// comes from a send whose deadline passes.
func newTimeoutError(t *testing.T) error {
	err := channel.NewMessage(0, nil).WithTimeout(time.Millisecond).SendAndWaitForWire(unwrittenSender{})
	require.True(t, channel.IsTimeout(err))
	return err
}

// stalledCtrlChannel holds every send until the send's context ends, as a channel with a full send queue does.
type stalledCtrlChannel struct {
	channel.Channel
}

func (self *stalledCtrlChannel) Send(s channel.Sendable) error {
	<-s.Context().Done()
	return s.Context().Err()
}

type nopPeer struct {
	xgress.Connection
}

func (self *nopPeer) Close() error {
	return nil
}

// newRoutedXgress returns an open xgress whose payloads the returned forwarder routes to dest, through
// the returned data plane.
func newRoutedXgress(t *testing.T, dest *busyDestination) (*xgress.Xgress, *forwarder.Forwarder, xgress.DataPlaneAdapter) {
	closeNotify := make(chan struct{})
	t.Cleanup(func() { close(closeNotify) })

	fwd := forwarder.NewForwarder(nil, nil, &env.ForwarderOptions{}, closeNotify)
	fwd.RegisterDestination(testCircuitId, testDstAddr, dest)
	require.NoError(t, fwd.Route("ctrl", &ctrl_pb.Route{
		CircuitId: testCircuitId,
		Forwards: []*ctrl_pb.Route_Forward{
			{SrcAddress: testSrcAddr, DstAddress: testDstAddr, DstType: ctrl_pb.DestType_Link},
		},
	}))

	x := xgress.NewXgress(testCircuitId, "ctrl", testSrcAddr, &nopPeer{}, xgress.Initiator, xgress.DefaultOptions(), nil)
	dataPlane := NewXgressDataPlaneAdapter(DataPlaneAdapterConfig{Forwarder: fwd})
	x.SetDataPlaneAdapter(dataPlane)
	return x, fwd, dataPlane
}

// TestCloseReturnsWhenNextHopAndControllerStall verifies that Close returns while the next hop times
// out every send and the controller takes no messages, making one end-of-circuit attempt from Close and
// one from the close handler.
func TestCloseReturnsWhenNextHopAndControllerStall(t *testing.T) {
	req := require.New(t)

	// A finite count lets a retrying Close finish, so a regression fails on the count rather than hanging.
	dest := &busyDestination{timeouts: 100, timeoutErr: newTimeoutError(t)}
	x, fwd, _ := newRoutedXgress(t, dest)
	x.AddCloseHandler(NewCloseHandler(&env.MockNetworkControllers{Channel: &stalledCtrlChannel{}}, fwd))

	closed := make(chan struct{})
	go func() {
		x.Close()
		close(closed)
	}()

	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		req.Fail("Close did not return while the next hop and controller were stalled")
	}
	req.Equal(int32(2), dest.attempts.Load())
}

// TestForwardPayloadRetriesTimeoutsWhileOpen verifies that a payload from an open xgress is retried
// until the next hop accepts it.
func TestForwardPayloadRetriesTimeoutsWhileOpen(t *testing.T) {
	req := require.New(t)

	dest := &busyDestination{timeouts: 3, timeoutErr: newTimeoutError(t)}
	x, _, dataPlane := newRoutedXgress(t, dest)
	payload := &xgress.Payload{CircuitId: testCircuitId, Data: []byte("data")}

	dataPlane.ForwardPayload(payload, x, context.Background())

	req.Equal(int32(4), dest.attempts.Load())
}
