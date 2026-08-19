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

package xgress_test

import (
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openziti/channel/v4"
	"github.com/openziti/sdk-golang/xgress"
	"github.com/openziti/ziti/router/xgress_common"
	"github.com/stretchr/testify/require"
)

// TestXgressClosesWhenPeerEndsWithoutHalfClose covers a circuit whose initiating connection has
// no half-close semantics, as datagram flows from an intercepting tunneler do. When such a
// connection reaches end of input it is finished for good, so the xgress must close and take the
// circuit with it.
//
// Treating that end of input as a half-close instead leaves the xgress waiting on an
// end-of-circuit its peer has no reason to send, so it is never closed, never reported to the
// controller, and holds its goroutines and buffers until the process exits.
func TestXgressClosesWhenPeerEndsWithoutHalfClose(t *testing.T) {
	req := require.New(t)

	adapter := NewMockDataPlaneAdapter()
	defer adapter.Close()

	circuitId := "teardown-circuit"
	initAddr := xgress.Address("init")
	termAddr := xgress.Address("term")

	options := xgress.DefaultOptions()

	// halfClose false: the flow has no way to signal a write-side close on its own
	endedFlow := &endedFlowConn{}
	initiatorConn := xgress_common.NewXgressConn(endedFlow, false, xgress_common.ConnTypeTunnel)
	terminatorConn := &blockingConn{closeNotify: make(chan struct{})}

	initiator := xgress.NewXgress(circuitId, "test", initAddr, initiatorConn, xgress.Initiator, options, nil)
	terminator := xgress.NewXgress(circuitId, "test", termAddr, terminatorConn, xgress.Terminator, options, nil)

	initiator.SetDataPlaneAdapter(adapter)
	terminator.SetDataPlaneAdapter(adapter)
	adapter.RegisterXgress(initiator)
	adapter.RegisterXgress(terminator)
	adapter.ConnectCircuit(circuitId, initAddr, termAddr)
	adapter.ConnectCircuit(circuitId, termAddr, initAddr)

	initiatorClosed := make(chan struct{})
	initiator.AddCloseHandler(xgress.CloseHandlerF(func(*xgress.Xgress) {
		close(initiatorClosed)
	}))

	initiator.Start()
	terminator.Start()

	select {
	case <-initiatorClosed:
	case <-time.After(30 * time.Second):
		req.Fail("initiating xgress was never closed after its connection ended")
	}

	req.True(initiator.IsClosed(), "initiating xgress should report closed")
	req.Equal(int32(1), endedFlow.closeCount.Load(),
		"closing the xgress should release the connection underneath it, exactly once")
}

// blockingConn is a peer that never produces or accepts anything, standing in for a hosted
// application that has no reason to speak first.
type blockingConn struct {
	closeNotify chan struct{}
	closed      bool
}

func (self *blockingConn) LogContext() string { return "blocking" }

func (self *blockingConn) ReadPayload() ([]byte, map[uint8][]byte, error) {
	<-self.closeNotify
	return nil, nil, io.EOF
}

func (self *blockingConn) WritePayload(b []byte, _ map[uint8][]byte) (int, error) {
	return len(b), nil
}

func (self *blockingConn) Close() error {
	if !self.closed {
		self.closed = true
		close(self.closeNotify)
	}
	return nil
}

func (self *blockingConn) HandleControlMsg(xgress.ControlType, channel.Headers, xgress.ControlReceiver) error {
	return nil
}

// endedFlowConn models a datagram flow whose association has expired. Reads report end of input,
// and the deadline setters succeed the way udp_vconn's no-op implementations do, so the liveness
// probe in ReadPayload cannot tell that the flow is gone.
type endedFlowConn struct {
	net.Conn
	closeCount atomic.Int32
}

func (self *endedFlowConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (self *endedFlowConn) Write(b []byte) (int, error)      { return len(b), nil }
func (self *endedFlowConn) Close() error                     { self.closeCount.Add(1); return nil }
func (self *endedFlowConn) LocalAddr() net.Addr              { return nil }
func (self *endedFlowConn) RemoteAddr() net.Addr             { return nil }
func (self *endedFlowConn) SetDeadline(time.Time) error      { return nil }
func (self *endedFlowConn) SetReadDeadline(time.Time) error  { return nil }
func (self *endedFlowConn) SetWriteDeadline(time.Time) error { return nil }
