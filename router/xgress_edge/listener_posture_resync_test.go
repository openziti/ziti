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

package xgress_edge

import (
	"testing"
	"time"

	sdkedge "github.com/openziti/sdk-golang/v2/ziti/edge"
	"github.com/stretchr/testify/require"
)

func Test_PostureResync(t *testing.T) {
	t.Run("the first request on a connection is answered", func(t *testing.T) {
		req := require.New(t)

		// arrange
		conn := newResyncTestConn(t)

		// act
		answered := conn.claimPostureResync()

		// assert
		req.True(answered, "the first request on a connection must be answered")
	})

	t.Run("requests inside the interval are not answered immediately", func(t *testing.T) {
		req := require.New(t)

		// arrange
		conn := newResyncTestConn(t)
		req.True(conn.claimPostureResync())

		// act
		second := conn.claimPostureResync()
		third := conn.claimPostureResync()

		// assert
		req.False(second, "a second request inside the interval must not be answered immediately")
		req.False(third, "a third request inside the interval must not be answered immediately")
	})

	// The SDK will not ask again until it is answered, so dropping the request would leave it
	// waiting indefinitely.
	t.Run("a rate limited request is deferred rather than dropped", func(t *testing.T) {
		req := require.New(t)

		// arrange
		conn := newResyncTestConn(t)
		req.True(conn.claimPostureResync())

		// act
		req.False(conn.claimPostureResync())

		// assert
		conn.posture.resync.Lock()
		deferred := conn.posture.resync.deferredSend
		conn.posture.resync.Unlock()
		req.NotNil(deferred, "a rate limited request must be deferred")
	})

	t.Run("repeated requests inside the interval share one deferred send", func(t *testing.T) {
		req := require.New(t)

		// arrange
		conn := newResyncTestConn(t)
		req.True(conn.claimPostureResync())
		req.False(conn.claimPostureResync())
		conn.posture.resync.Lock()
		first := conn.posture.resync.deferredSend
		conn.posture.resync.Unlock()
		req.NotNil(first)

		// act
		for range 50 {
			req.False(conn.claimPostureResync())
		}

		// assert
		conn.posture.resync.Lock()
		deferred := conn.posture.resync.deferredSend
		conn.posture.resync.Unlock()
		req.Same(first, deferred, "a burst inside one interval must leave a single pending send, not one per request")
	})

	t.Run("a request after the interval is answered", func(t *testing.T) {
		req := require.New(t)

		// arrange: move the last answer back by the interval rather than sleeping through it
		conn := newResyncTestConn(t)
		req.True(conn.claimPostureResync())
		conn.posture.resync.Lock()
		conn.posture.resync.lastSentAt = time.Now().Add(-minPostureResyncInterval)
		conn.posture.resync.Unlock()

		// act
		answered := conn.claimPostureResync()

		// assert
		req.True(answered, "a request after the interval must be answered")
	})

	t.Run("the deferred send clears its pending state", func(t *testing.T) {
		req := require.New(t)

		// arrange: the connection has no api session, so the send itself does nothing
		conn := newResyncTestConn(t)
		req.True(conn.claimPostureResync())
		req.False(conn.claimPostureResync())

		// act
		conn.sendDeferredPostureResync()

		// assert
		conn.posture.resync.Lock()
		deferred := conn.posture.resync.deferredSend
		conn.posture.resync.Unlock()
		req.Nil(deferred, "the deferred send must clear itself, so a later request can be deferred again")
	})

	t.Run("cancelling stops a pending deferred send", func(t *testing.T) {
		req := require.New(t)

		// arrange
		conn := newResyncTestConn(t)
		req.True(conn.claimPostureResync())
		req.False(conn.claimPostureResync())

		// act
		conn.cancelDeferredPostureResync()

		// assert
		conn.posture.resync.Lock()
		deferred := conn.posture.resync.deferredSend
		conn.posture.resync.Unlock()
		req.Nil(deferred, "closing must not leave a timer holding the connection")
	})

	t.Run("cancelling with nothing pending is harmless", func(t *testing.T) {
		req := require.New(t)

		// arrange: the common case, a connection closing having never been asked for a resync
		conn := newResyncTestConn(t)

		// act and assert
		req.NotPanics(conn.cancelDeferredPostureResync, "cancelling with no deferred send must do nothing")
	})
}

// newResyncTestConn returns a connection with an active service subscription and no api session.
// A resync it defers is cancelled when the test ends.
func newResyncTestConn(t *testing.T) *edgeClientConn {
	t.Helper()

	conn := &edgeClientConn{ch: sdkedge.NewSingleSdkChannel(&recordingTestChannel{})}
	conn.svcSubscription.Lock()
	conn.svcSubscription.active = true
	conn.svcSubscription.Unlock()
	t.Cleanup(conn.cancelDeferredPostureResync)
	return conn
}

func (self *recordingTestChannel) IsClosed() bool {
	return false
}
