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

	"github.com/openziti/ziti/v2/controller/change"
	"github.com/stretchr/testify/require"
)

// TestRouterDeletedCleanups_CoverEveryPerRouterStore is the guard on the defect that let canary state leak:
// a per-router store was added, wired into the epoch sweep's registry only, and nothing cleaned it up when a
// router was deleted. Adding a store without registering its cleanup must fail here rather than leak until
// someone reads a memory profile.
func TestRouterDeletedCleanups_CoverEveryPerRouterStore(t *testing.T) {
	_, network, _ := newConnectTestNetwork(t)

	registered := map[string]bool{}
	for _, name := range network.routerDeletedCleanupNames() {
		registered[name] = true
	}

	// Every gossip store keyed by owner holds per-router state, whether or not it takes part in the epoch
	// sweep. Canaries deliberately sit out the sweep, since they carry the epoch that detects a change, and
	// that exclusion is exactly what hid their missing delete cleanup.
	for _, storeType := range []string{LinkGossipStoreType, LinkMetricsGossipStoreType, CanaryGossipStoreType} {
		require.True(t, registered[storeType],
			"gossip store %q holds per-router state and must register router-deleted cleanup", storeType)
	}

	require.True(t, registered["linkIndex"], "the link index is keyed by router id and must be cleaned up")
}

// TestRouterDeleted_DropsCanaryStateOfAGoneId: a deleted router must not leave canary state behind, which
// is what grew without bound under router churn. Canary state is tracked outside the gossip store as well,
// so the cleanup has to drop both.
func TestRouterDeleted_DropsCanaryStateOfAGoneId(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	gone := newPersistedRouter(t, network, addr, "r0")
	network.canaryListener.EntryChanged(gone.Id, &CanaryValue{Seq: 3}, 1, gone.Id, false, 0)
	_, found := network.GetCanaryForRouter(gone.Id)
	require.True(t, found)

	require.NoError(t, network.Router.Delete(gone.Id, change.New()))
	network.runRouterDeletedCleanups(gone.Id)

	_, found = network.GetCanaryForRouter(gone.Id)
	require.False(t, found, "a deleted router's canary state must not be left behind")
}

// TestRouterDeleted_DropsGossipOwnerOfAGoneId: a deleted router must have its gossip owner data dropped,
// which is what keeps the store from growing with every router ever deleted.
func TestRouterDeleted_DropsGossipOwnerOfAGoneId(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	gone := newPersistedRouter(t, network, addr, "r0")
	require.NoError(t, network.CanaryGossipType.Set("r0", gone.Id, &CanaryValue{Seq: 1}))
	_, _, ok := network.CanaryGossipType.GetForOwner(gone.Id, "r0")
	require.True(t, ok)

	require.NoError(t, network.Router.Delete(gone.Id, change.New()))
	network.runRouterDeletedCleanups(gone.Id)

	_, _, ok = network.CanaryGossipType.GetForOwner(gone.Id, "r0")
	require.False(t, ok, "a deleted router's gossip entries must not be left behind")
}
