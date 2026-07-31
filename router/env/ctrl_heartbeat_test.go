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
	"sync/atomic"
	"testing"
	"time"

	"github.com/openziti/ziti/v2/common/ctrlchan"
	"github.com/stretchr/testify/require"
)

// heartbeatTestChannel is a ctrlchan.CtrlChannel double that records Close calls. It embeds the interface,
// so any method the heartbeat path does not use panics rather than silently returning a zero value.
type heartbeatTestChannel struct {
	ctrlchan.CtrlChannel
	closed atomic.Bool
}

func (self *heartbeatTestChannel) Close() error {
	self.closed.Store(true)
	return nil
}

func (self *heartbeatTestChannel) IsClosed() bool { return self.closed.Load() }

func (self *heartbeatTestChannel) IsConnected() bool { return !self.closed.Load() }

func newHeartbeatTestCtrl(t *testing.T) (*networkCtrl, *heartbeatTestChannel) {
	t.Helper()
	ch := &heartbeatTestChannel{}
	options := NewDefaultHeartbeatOptions()
	options.CloseUnresponsiveTimeout = time.Minute
	options.UnresponsiveAfter = 5 * time.Second
	return newNetworkCtrl(ch, "tls:localhost:6262", options), ch
}

// TestCheckHeartBeat_ClosesWhenNoResponse covers the recovery path for a control channel whose peer has
// stopped answering. Until it is closed, the channel keeps its group and only ever dials additional
// underlays, which the controller refuses once it has torn its side of the group down, so the router can
// never re-establish it.
func TestCheckHeartBeat_ClosesWhenNoResponse(t *testing.T) {
	ctrl, ch := newHeartbeatTestCtrl(t)

	// A fresh channel is not judged unresponsive before its first heartbeat could be answered.
	ctrl.CheckHeartBeat()
	require.False(t, ch.IsClosed(), "a channel that has just connected must not be closed")

	// No response for longer than the timeout.
	ctrl.lastRx = time.Now().Add(-2 * time.Minute).UnixMilli()
	ctrl.CheckHeartBeat()
	require.True(t, ch.IsClosed(), "a channel with no heartbeat response in time must be closed")
}

// TestCheckHeartBeat_SlowButAnsweringIsNotClosed guards the control channel's preference for staying up:
// high latency only deprioritizes a controller when choosing between them. Closing a channel that is
// merely slow would add control-plane downtime, so only a peer that has stopped answering entirely is
// torn down.
func TestCheckHeartBeat_SlowButAnsweringIsNotClosed(t *testing.T) {
	ctrl, ch := newHeartbeatTestCtrl(t)

	// Answering, but slowly enough to count as unresponsive for selection purposes.
	ctrl.lastRx = time.Now().UnixMilli()
	ctrl.latency.Store(int64(30 * time.Second))

	ctrl.CheckHeartBeat()

	require.True(t, ctrl.IsUnresponsive(), "high latency should mark the controller unresponsive")
	require.False(t, ch.IsClosed(), "a channel that is still answering must not be closed")
}

// TestCheckHeartBeat_ZeroTimeoutDisablesClose: a zero timeout turns the teardown off, for deployments that
// would rather keep a silent channel than have it recycled.
func TestCheckHeartBeat_ZeroTimeoutDisablesClose(t *testing.T) {
	ctrl, ch := newHeartbeatTestCtrl(t)
	ctrl.heartbeatOptions.CloseUnresponsiveTimeout = 0

	ctrl.lastRx = time.Now().Add(-time.Hour).UnixMilli()
	ctrl.CheckHeartBeat()

	require.False(t, ch.IsClosed(), "a zero close timeout must disable the teardown")
}
