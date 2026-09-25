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

	"github.com/openziti/channel/v4"
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

// TestCheckHeartBeat_TimeoutWindowRestartsAfterReconnect: a reconnect restarts the response window.
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
	require.Less(t, ctrl.timeSinceLastResponse(), time.Minute)
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
	ctrl.lastRx = time.Now().Add(-2 * time.Minute).UnixMilli()
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

// TestNewHeartbeatOptions_KeysLandInOwnFields covers the overlay this package puts on top of the channel
// heartbeat options. The close timeout is only meaningful if it survives config loading, and a value that
// lands in checkInterval instead makes the pulse sample a stale last-response time, which closes healthy
// channels rather than unresponsive ones.
func TestNewHeartbeatOptions_KeysLandInOwnFields(t *testing.T) {
	load := func(t *testing.T, src map[interface{}]interface{}) *HeartbeatOptions {
		t.Helper()
		chOptions, err := channel.LoadHeartbeatOptions(src)
		require.NoError(t, err)
		options, err := NewHeartbeatOptions(chOptions)
		require.NoError(t, err)
		return options
	}

	t.Run("each key lands in its own field", func(t *testing.T) {
		options := load(t, map[interface{}]interface{}{
			"sendInterval":             "3s",
			"checkInterval":            "4s",
			"closeUnresponsiveTimeout": "5s",
			"unresponsiveAfter":        "6s",
		})
		require.Equal(t, 3*time.Second, options.SendInterval)
		require.Equal(t, 4*time.Second, options.CheckInterval)
		require.Equal(t, 5*time.Second, options.CloseUnresponsiveTimeout)
		require.Equal(t, 6*time.Second, options.UnresponsiveAfter)
	})

	t.Run("the close timeout leaves the check interval alone", func(t *testing.T) {
		defaults := NewDefaultHeartbeatOptions()
		options := load(t, map[interface{}]interface{}{
			"closeUnresponsiveTimeout": "90s",
		})
		require.Equal(t, 90*time.Second, options.CloseUnresponsiveTimeout)
		require.Equal(t, defaults.CheckInterval, options.CheckInterval)
		require.Equal(t, defaults.UnresponsiveAfter, options.UnresponsiveAfter)
	})

	t.Run("the teardown can be disabled from config", func(t *testing.T) {
		options := load(t, map[interface{}]interface{}{
			"closeUnresponsiveTimeout": "0s",
		})
		require.Zero(t, options.CloseUnresponsiveTimeout)
		require.Positive(t, options.CheckInterval, "a zero close timeout must not zero the check interval")
	})

	t.Run("defaults apply when nothing is set", func(t *testing.T) {
		defaults := NewDefaultHeartbeatOptions()
		options := load(t, map[interface{}]interface{}{})
		require.Equal(t, defaults.SendInterval, options.SendInterval)
		require.Equal(t, defaults.CheckInterval, options.CheckInterval)
		require.Equal(t, defaults.CloseUnresponsiveTimeout, options.CloseUnresponsiveTimeout)
		require.Equal(t, defaults.UnresponsiveAfter, options.UnresponsiveAfter)
	})
}

// TestNewHeartbeatOptions_RejectsUnworkableCadence covers the cadence relation the loader enforces.
// CheckHeartBeat only runs from the heartbeat pulse, so the check interval is the sampling rate for
// the close timeout: at or above it the check reads a response time up to a full interval stale and
// tears down a controller that is answering, which then reconnects and repeats.
func TestNewHeartbeatOptions_RejectsUnworkableCadence(t *testing.T) {
	load := func(src map[interface{}]interface{}) (*HeartbeatOptions, error) {
		chOptions, err := channel.LoadHeartbeatOptions(src)
		require.NoError(t, err)
		return NewHeartbeatOptions(chOptions)
	}

	// The close timeout defaults to 30s, so raising only the check interval is enough to break it.
	t.Run("a check interval above the default timeout is rejected", func(t *testing.T) {
		_, err := load(map[interface{}]interface{}{
			"checkInterval": "60s",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "checkInterval")
	})

	t.Run("a check interval above an explicit timeout is rejected", func(t *testing.T) {
		_, err := load(map[interface{}]interface{}{
			"checkInterval":            "60s",
			"closeUnresponsiveTimeout": "30s",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "checkInterval")
	})

	t.Run("a send interval above the timeout is rejected", func(t *testing.T) {
		_, err := load(map[interface{}]interface{}{
			"sendInterval":             "60s",
			"closeUnresponsiveTimeout": "30s",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "sendInterval")
	})

	t.Run("a zero check interval is rejected", func(t *testing.T) {
		_, err := load(map[interface{}]interface{}{
			"checkInterval": "0s",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "checkInterval")
	})

	t.Run("widening the timeout alongside the check interval is accepted", func(t *testing.T) {
		options, err := load(map[interface{}]interface{}{
			"checkInterval":            "60s",
			"sendInterval":             "60s",
			"closeUnresponsiveTimeout": "5m",
		})
		require.NoError(t, err)
		require.Equal(t, time.Minute, options.CheckInterval)
		require.Equal(t, 5*time.Minute, options.CloseUnresponsiveTimeout)
	})

	t.Run("disabling the teardown accepts any positive check interval", func(t *testing.T) {
		options, err := load(map[interface{}]interface{}{
			"checkInterval":            "60s",
			"closeUnresponsiveTimeout": "0s",
		})
		require.NoError(t, err)
		require.Zero(t, options.CloseUnresponsiveTimeout)
	})
}
