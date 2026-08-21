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

package network

import (
	"testing"

	"github.com/openziti/ziti/v2/controller/model"
	"github.com/stretchr/testify/require"
)

// TestSyncStates_DiscardsUpdatesForClosedChannel covers the retry behaviour for peer state changes bound
// for a router whose control channel has closed while it is still in the connected map. Sending on a
// closed channel fails immediately instead of blocking, and a failed send is retried as soon as the event
// loop turns, so retaining the updates spins the loop and floods the log. Reconnecting resyncs everything,
// so the pending changes are discarded instead. Updates for a router with a live channel must still be
// retained.
func TestSyncStates_DiscardsUpdatesForClosedChannel(t *testing.T) {
	ctx, network, addr := newConnectTestNetwork(t)

	// A standalone instance, so syncStates can be driven directly; the Network's own RouterMessaging runs
	// an event loop goroutine that would race these map accesses.
	rm := NewRouterMessaging(ctx, network.RouterMessaging.routerCommPool)

	deadCh := &fakeCtrlChannel{}
	dead := model.NewRouterForTest("r1", "", addr, deadCh, 0, false)
	network.Router.MarkConnected(dead)
	// The channel closes without any teardown running, so it stays in the connected map.
	deadCh.closed.Store(true)
	rm.routerUpdates["r1"] = &routerUpdates{changedRouters: map[string]struct{}{"other": {}}}

	// A live router with a send already in flight, which syncStates skips without queueing another.
	liveCh := &fakeCtrlChannel{}
	live := model.NewRouterForTest("r2", "", addr, liveCh, 0, false)
	network.Router.MarkConnected(live)
	liveUpdates := &routerUpdates{changedRouters: map[string]struct{}{"other": {}}}
	liveUpdates.sendInProgress.Store(true)
	rm.routerUpdates["r2"] = liveUpdates

	rm.syncStates()

	require.NotContains(t, rm.routerUpdates, "r1", "updates for a closed channel must be discarded, not retried")
	require.Contains(t, rm.routerUpdates, "r2", "updates for a live channel must be retained")
}
