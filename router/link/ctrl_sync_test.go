package link

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ctrlSynchronizer(t *testing.T) {
	t.Run("an unknown controller is synced, owes no refresh, and owes the listener set", func(t *testing.T) {
		sync := newCtrlSynchronizer()
		require.True(t, sync.isSynced("ctrl1"))
		_, ok := sync.beginRefresh("ctrl1")
		require.False(t, ok)
		require.True(t, sync.isListenersSendPending("ctrl1"))
	})

	t.Run("a reconnect owes a refresh, claimed once, and sync follows the write", func(t *testing.T) {
		sync := newCtrlSynchronizer()
		sync.markReconnected("ctrl1")
		require.False(t, sync.isSynced("ctrl1"))

		gen, ok := sync.beginRefresh("ctrl1")
		require.True(t, ok)
		_, again := sync.beginRefresh("ctrl1")
		require.False(t, again, "one refresh in flight at a time")

		require.True(t, sync.markRefreshWritten("ctrl1", gen))
		require.True(t, sync.isSynced("ctrl1"))
		_, owed := sync.beginRefresh("ctrl1")
		require.False(t, owed, "nothing owed after the write")
	})

	t.Run("a failed refresh releases the claim and leaves the debt", func(t *testing.T) {
		sync := newCtrlSynchronizer()
		sync.markReconnected("ctrl1")
		_, ok := sync.beginRefresh("ctrl1")
		require.True(t, ok)
		sync.markRefreshFailed("ctrl1")
		require.False(t, sync.isRefreshInFlight("ctrl1"))
		_, ok = sync.beginRefresh("ctrl1")
		require.True(t, ok)
	})

	t.Run("a reconnect during a refresh invalidates it", func(t *testing.T) {
		sync := newCtrlSynchronizer()
		sync.markReconnected("ctrl1")
		gen, _ := sync.beginRefresh("ctrl1")
		sync.markReconnected("ctrl1")
		require.False(t, sync.markRefreshWritten("ctrl1", gen), "a refresh built before the later reconnect does not satisfy it")
		require.False(t, sync.isSynced("ctrl1"))
		gen2, ok := sync.beginRefresh("ctrl1")
		require.True(t, ok)
		require.True(t, sync.markRefreshWritten("ctrl1", gen2))
		require.True(t, sync.isSynced("ctrl1"))
	})

	t.Run("listeners sent for the current generation clears the debt, an older generation does not", func(t *testing.T) {
		sync := newCtrlSynchronizer()
		gen := sync.currentListenersGen()
		sync.markListenersChanged()
		sync.markListenersSent("ctrl1", gen, sync.reconnectGen("ctrl1"))
		require.True(t, sync.isListenersSendPending("ctrl1"))
		sync.markListenersSent("ctrl1", sync.currentListenersGen(), sync.reconnectGen("ctrl1"))
		require.False(t, sync.isListenersSendPending("ctrl1"))
	})

	t.Run("a reconnect makes the listener set owed again", func(t *testing.T) {
		sync := newCtrlSynchronizer()
		sync.markListenersSent("ctrl1", sync.currentListenersGen(), sync.reconnectGen("ctrl1"))
		require.False(t, sync.isListenersSendPending("ctrl1"))
		sync.markReconnected("ctrl1")
		require.True(t, sync.isListenersSendPending("ctrl1"))
	})

	t.Run("a listener send written before a reconnect does not satisfy the new connection", func(t *testing.T) {
		sync := newCtrlSynchronizer()
		sync.markReconnected("ctrl1")
		gen, _ := sync.beginRefresh("ctrl1")
		require.True(t, sync.markRefreshWritten("ctrl1", gen))

		listenersGen := sync.currentListenersGen()
		staleReconnectGen := sync.reconnectGen("ctrl1")

		sync.markReconnected("ctrl1") // reconnect after the write, before its completion
		sync.markListenersSent("ctrl1", listenersGen, staleReconnectGen)
		require.True(t, sync.isListenersSendPending("ctrl1"), "the stale completion must not mark the new connection sent")

		sync.markListenersSent("ctrl1", listenersGen, sync.reconnectGen("ctrl1"))
		require.False(t, sync.isListenersSendPending("ctrl1"), "a send fenced with the current generation clears the debt")
	})

	t.Run("forget drops only settled controllers no longer known", func(t *testing.T) {
		sync := newCtrlSynchronizer()

		// ctrl1: settled, gone from the snapshot -> dropped.
		sync.markReconnected("ctrl1")
		gen, _ := sync.beginRefresh("ctrl1")
		require.True(t, sync.markRefreshWritten("ctrl1", gen))
		sync.markListenersSent("ctrl1", sync.currentListenersGen(), sync.reconnectGen("ctrl1"))

		// ctrl2: connected after the snapshot was taken, so absent from it, but owing a refresh -> kept.
		sync.markReconnected("ctrl2")

		// ctrl3: refreshed but still owing the listener set -> kept.
		sync.markReconnected("ctrl3")
		gen3, _ := sync.beginRefresh("ctrl3")
		require.True(t, sync.markRefreshWritten("ctrl3", gen3))

		sync.forget(map[string]struct{}{})

		_, ok1 := sync.states["ctrl1"]
		require.False(t, ok1, "a settled state for an unknown controller is dropped")
		require.False(t, sync.isSynced("ctrl2"), "a reconnect recorded after the snapshot keeps its refresh debt")
		_, owed := sync.beginRefresh("ctrl2")
		require.True(t, owed)
		require.True(t, sync.isListenersSendPending("ctrl3"), "an owed listener set is kept")
	})
}
