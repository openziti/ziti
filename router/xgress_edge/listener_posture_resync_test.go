package xgress_edge

import (
	"testing"
	"time"

	sdkedge "github.com/openziti/sdk-golang/v2/ziti/edge"
	"github.com/stretchr/testify/require"
)

func (self *recordingTestChannel) IsClosed() bool {
	return false
}

func newResyncTestConn() *edgeClientConn {
	conn := &edgeClientConn{ch: sdkedge.NewSingleSdkChannel(&recordingTestChannel{})}
	conn.svcSubscription.Lock()
	conn.svcSubscription.active = true
	conn.svcSubscription.Unlock()
	return conn
}

func Test_PostureResync_FirstRequestIsAnswered(t *testing.T) {
	req := require.New(t)
	conn := newResyncTestConn()

	req.True(conn.claimPostureResync(), "the first request on a connection must be answered")
}

func Test_PostureResync_RequestsInsideIntervalAreRateLimited(t *testing.T) {
	req := require.New(t)
	conn := newResyncTestConn()

	req.True(conn.claimPostureResync())
	req.False(conn.claimPostureResync(), "a second request inside the interval must not be answered immediately")
	req.False(conn.claimPostureResync())
}

func Test_PostureResync_RateLimitedRequestIsDeferredNotDropped(t *testing.T) {
	req := require.New(t)
	conn := newResyncTestConn()

	req.True(conn.claimPostureResync())
	req.False(conn.claimPostureResync())

	conn.posture.resync.Lock()
	deferred := conn.posture.resync.deferredSend
	conn.posture.resync.Unlock()

	req.NotNil(deferred, "a request that is rate limited must be deferred, since the SDK will not ask again")
}

func Test_PostureResync_RepeatedRequestsCollapseIntoOneDeferredSend(t *testing.T) {
	req := require.New(t)
	conn := newResyncTestConn()

	req.True(conn.claimPostureResync())
	for range 50 {
		req.False(conn.claimPostureResync())
	}

	// a burst inside one interval must leave a single pending send, not one per request
	conn.posture.resync.Lock()
	deferred := conn.posture.resync.deferredSend
	conn.posture.resync.Unlock()
	req.NotNil(deferred)
}

func Test_PostureResync_AnsweredAgainAfterInterval(t *testing.T) {
	req := require.New(t)
	conn := newResyncTestConn()

	req.True(conn.claimPostureResync())

	// simulate the interval having elapsed rather than sleeping through it
	conn.posture.resync.Lock()
	conn.posture.resync.lastSentAt = time.Now().Add(-minPostureResyncInterval)
	conn.posture.resync.Unlock()

	req.True(conn.claimPostureResync(), "a request after the interval must be answered")
}

func Test_PostureResync_DeferredSendClearsPendingState(t *testing.T) {
	req := require.New(t)
	conn := newResyncTestConn()

	req.True(conn.claimPostureResync())
	req.False(conn.claimPostureResync())

	// the connection has no api session, so the send itself is a no-op; what matters is that the
	// deferred flag is cleared so a later gap can be reported again
	conn.sendDeferredPostureResync()

	conn.posture.resync.Lock()
	deferred := conn.posture.resync.deferredSend
	conn.posture.resync.Unlock()
	req.Nil(deferred)
}

func Test_PostureResync_CancelStopsPendingDeferredSend(t *testing.T) {
	req := require.New(t)
	conn := newResyncTestConn()

	req.True(conn.claimPostureResync())
	req.False(conn.claimPostureResync())

	conn.cancelDeferredPostureResync()

	conn.posture.resync.Lock()
	deferred := conn.posture.resync.deferredSend
	conn.posture.resync.Unlock()

	req.Nil(deferred, "closing must not leave a timer holding the connection")
}

func Test_PostureResync_CancelWithNothingPendingIsSafe(t *testing.T) {
	conn := newResyncTestConn()

	// the common case: a connection closes having never been asked for a resync
	conn.cancelDeferredPostureResync()
	conn.cancelDeferredPostureResync()
}
