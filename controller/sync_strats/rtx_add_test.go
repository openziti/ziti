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

package sync_strats

import (
	"testing"

	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/stretchr/testify/require"
)

// TestRouterTxMap_AddStopsReplaced verifies routerTxMap.Add stops the RouterSender it replaces. This is
// required under reject-if-busy: on a reconnect/takeover the new connection's RouterConnected can Add its
// sender before the old connection's RouterDisconnected runs (the broker dispatches it asynchronously),
// so without this the replaced sender's goroutine would be orphaned.
func TestRouterTxMap_AddStopsReplaced(t *testing.T) {
	m := &routerTxMap{internalMap: cmap.New[*RouterSender]()}

	old := &RouterSender{closeNotify: make(chan struct{})}
	old.running.Store(true)
	m.Add("r1", old)

	newRtx := &RouterSender{closeNotify: make(chan struct{})}
	newRtx.running.Store(true)
	m.Add("r1", newRtx)

	require.False(t, old.running.Load(), "replaced sender should be stopped")
	select {
	case <-old.closeNotify:
	default:
		t.Fatal("replaced sender's closeNotify should be closed")
	}
	require.True(t, newRtx.running.Load(), "installed sender should still be running")
	require.Equal(t, newRtx, m.Get("r1"))

	// Re-adding the same instance must not stop it.
	m.Add("r1", newRtx)
	require.True(t, newRtx.running.Load(), "re-adding the same sender must not stop it")
}
