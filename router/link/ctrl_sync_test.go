package link

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ctrlSynchronizer(t *testing.T) {
	t.Run("an unknown controller is synced and owes no refresh", func(t *testing.T) {
		sync := newCtrlSynchronizer()
		require.True(t, sync.isSynced("ctrl1"))
		_, ok := sync.beginRefresh("ctrl1")
		require.False(t, ok)
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

	t.Run("forget drops only settled controllers no longer known", func(t *testing.T) {
		sync := newCtrlSynchronizer()

		// ctrl1: settled, gone from the snapshot -> dropped.
		sync.markReconnected("ctrl1")
		gen, _ := sync.beginRefresh("ctrl1")
		require.True(t, sync.markRefreshWritten("ctrl1", gen))

		// ctrl2: connected after the snapshot was taken, so absent from it, but owing a refresh -> kept.
		sync.markReconnected("ctrl2")

		sync.forget(map[string]struct{}{})

		_, ok1 := sync.states["ctrl1"]
		require.False(t, ok1, "a settled state for an unknown controller is dropped")
		require.False(t, sync.isSynced("ctrl2"), "a reconnect recorded after the snapshot keeps its refresh debt")
		_, owed := sync.beginRefresh("ctrl2")
		require.True(t, owed)
	})
}
