package xgress_edge

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Test_StateListenerPinnedWhileSubscriptionActive locks in that an active service push
// subscription pins the RDM identity listener: an otherwise idle connection (no conns, no
// terminators, idle past the removal window) must not become eligible for listener removal while
// the SDK relies on it for push, or push dies silently while the SDK has polling paused.
func Test_StateListenerPinnedWhileSubscriptionActive(t *testing.T) {
	conn := &edgeClientConn{}
	conn.stateListener.enabled.Store(true)
	conn.stateListener.lastRequired.Store(time.Now().Add(-time.Hour))

	conn.svcSubscription.Lock()
	conn.svcSubscription.active = true
	conn.svcSubscription.Unlock()

	require.False(t, conn.IsStateListenerEligibleForRemovalCheck(),
		"an active push subscription must pin the RDM identity listener")

	conn.svcSubscription.Lock()
	conn.svcSubscription.active = false
	conn.svcSubscription.Unlock()

	require.True(t, conn.IsStateListenerEligibleForRemovalCheck(),
		"once the subscription is inactive the idle listener is removable again")
}
