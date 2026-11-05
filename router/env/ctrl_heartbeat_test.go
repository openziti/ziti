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

// heartbeatTestChannel is a ctrlchan.CtrlChannel double that records Close and Reconnect calls. It
// embeds the interface, so any method the heartbeat path does not use panics rather than silently
// returning a zero value.
type heartbeatTestChannel struct {
	ctrlchan.CtrlChannel
	closed     atomic.Bool
	reconnects atomic.Int32
}

func (self *heartbeatTestChannel) Close() error {
	self.closed.Store(true)
	return nil
}

func (self *heartbeatTestChannel) Reconnect() error {
	self.reconnects.Add(1)
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

// TestCheckHeartBeat_ReconnectsWhenNoResponse covers the recovery path for a control channel whose
// peer has stopped answering. Its underlays are closed so the dial policy re-establishes the group, but
// the channel itself must survive: closing it would drop the controller registration, and a router that
// has no controller details for the peer would then never redial.
func TestCheckHeartBeat_ReconnectsWhenNoResponse(t *testing.T) {
	ctrl, ch := newHeartbeatTestCtrl(t)

	// A fresh channel is not judged unresponsive before its first heartbeat could be answered.
	ctrl.CheckHeartBeat()
	require.Zero(t, ch.reconnects.Load(), "a channel that has just connected must be left alone")

	// No response for longer than the timeout.
	ctrl.lastRx = time.Now().Add(-2 * time.Minute).UnixMilli()
	ctrl.CheckHeartBeat()
	require.EqualValues(t, 1, ch.reconnects.Load(), "a channel with no heartbeat response in time must be reconnected")
	require.False(t, ch.IsClosed(), "the channel itself must stay open so its registration survives")
}

// TestCheckHeartBeat_TimeoutWindowRestartsAfterReconnect: a reconnect starts a timeout window of its own.
// Without that, every check until the next answered heartbeat would act again on the underlays that had
// just been re-established, and the channel could never come back.
func TestCheckHeartBeat_TimeoutWindowRestartsAfterReconnect(t *testing.T) {
	ctrl, ch := newHeartbeatTestCtrl(t)

	ctrl.lastRx = time.Now().Add(-2 * time.Minute).UnixMilli()
	ctrl.CheckHeartBeat()
	require.EqualValues(t, 1, ch.reconnects.Load())

	ctrl.CheckHeartBeat()
	require.EqualValues(t, 1, ch.reconnects.Load(), "a check immediately after a reconnect must not act again")
	require.False(t, ch.IsClosed())
}

// TestCheckHeartBeat_ReconnectIsNotAResponse: a reconnect is not an answer from the controller. Until one
// arrives the controller stays unresponsive, so it isn't offered for requests it can't serve.
func TestCheckHeartBeat_ReconnectIsNotAResponse(t *testing.T) {
	ctrl, ch := newHeartbeatTestCtrl(t)

	ctrl.lastRx = time.Now().Add(-2 * time.Minute).UnixMilli()
	ctrl.lastTx = time.Now().Add(-10 * time.Second).UnixMilli()
	ctrl.CheckHeartBeat()
	require.EqualValues(t, 1, ch.reconnects.Load())

	ctrl.CheckHeartBeat()
	require.True(t, ctrl.IsUnresponsive(), "a controller that has not answered since the reconnect must stay unresponsive")
	require.Greater(t, ctrl.timeSinceLastResponse(), time.Minute, "the reconnect must not be recorded as a response")
}

// TestCheckHeartBeat_ConsecutiveTimeoutCloses: a reconnect that has not produced an answered heartbeat by
// the next timeout is not going to. The controller is still gone, or a dead underlay is stuck in the
// channel and keeps every redial on the old group; either way only a channel that shares nothing with
// this one can recover, so the check closes it and leaves the redial to the close handler.
func TestCheckHeartBeat_ConsecutiveTimeoutCloses(t *testing.T) {
	ctrl, ch := newHeartbeatTestCtrl(t)

	ctrl.lastRx = time.Now().Add(-2 * time.Minute).UnixMilli()
	ctrl.CheckHeartBeat()
	require.EqualValues(t, 1, ch.reconnects.Load())
	require.False(t, ch.IsClosed(), "the first timeout reconnects rather than closes")

	// Another full window with no answer.
	ctrl.reconnectedAt.Store(time.Now().Add(-2 * time.Minute).UnixMilli())
	ctrl.CheckHeartBeat()
	require.True(t, ch.IsClosed(), "a timeout following an unanswered reconnect must close the channel")
	require.EqualValues(t, 1, ch.reconnects.Load(), "the second timeout must not reconnect again")
}

// TestCheckHeartBeat_AnsweredHeartbeatResetsEscalation: an answered heartbeat after a reconnect means the
// reconnect worked, so a later timeout starts over with a reconnect rather than closing.
func TestCheckHeartBeat_AnsweredHeartbeatResetsEscalation(t *testing.T) {
	ctrl, ch := newHeartbeatTestCtrl(t)

	ctrl.lastRx = time.Now().Add(-2 * time.Minute).UnixMilli()
	ctrl.CheckHeartBeat()
	require.EqualValues(t, 1, ch.reconnects.Load())

	// The rebuilt group answers a heartbeat.
	ctrl.HeartbeatRespRx(time.Now().UnixNano())

	ctrl.lastRx = time.Now().Add(-2 * time.Minute).UnixMilli()
	ctrl.CheckHeartBeat()
	require.EqualValues(t, 2, ch.reconnects.Load(), "a timeout after an answered heartbeat reconnects again")
	require.False(t, ch.IsClosed())
}

// TestCheckHeartBeat_SlowButAnsweringIsNotClosed guards the control channel's preference for staying up:
// high latency only deprioritizes a controller when choosing between them. Recycling a channel that is
// merely slow would add control-plane downtime, so only a peer that has stopped answering entirely is
// torn down.
func TestCheckHeartBeat_SlowButAnsweringIsNotClosed(t *testing.T) {
	ctrl, ch := newHeartbeatTestCtrl(t)

	// Answering, but slowly enough to count as unresponsive for selection purposes.
	ctrl.lastRx = time.Now().UnixMilli()
	ctrl.latency.Store(int64(30 * time.Second))

	ctrl.CheckHeartBeat()

	require.True(t, ctrl.IsUnresponsive(), "high latency should mark the controller unresponsive")
	require.Zero(t, ch.reconnects.Load(), "a channel that is still answering must be left alone")
	require.False(t, ch.IsClosed())
}

// TestCheckHeartBeat_ZeroTimeoutDisablesClose: a zero timeout turns the teardown off, for deployments that
// would rather keep a silent channel than have it recycled.
func TestCheckHeartBeat_ZeroTimeoutDisablesClose(t *testing.T) {
	ctrl, ch := newHeartbeatTestCtrl(t)
	ctrl.heartbeatOptions.CloseUnresponsiveTimeout = 0

	ctrl.lastRx = time.Now().Add(-time.Hour).UnixMilli()
	ctrl.CheckHeartBeat()

	require.Zero(t, ch.reconnects.Load(), "a zero close timeout must disable the teardown")
	require.False(t, ch.IsClosed())
}
