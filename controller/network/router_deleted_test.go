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

	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/model"
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

// TestRouterDeleted_RemovesItsLinks covers what a fleet that churns routers accumulated: links to a router
// that no longer exists.
//
// The cleanup dropped the deleted router's index but left the links themselves in the link table and in the
// surviving endpoint's set, where nothing would ever remove them. A deleted router's id never returns,
// since a recreated router is assigned a new one, so those links could not be cleared by anything later.
func TestRouterDeleted_RemovesItsLinks(t *testing.T) {
	req := require.New(t)
	_, network, addr := newConnectTestNetwork(t)

	gone := newPersistedRouter(t, network, addr, "gone")
	survivor := newPersistedRouter(t, network, addr, "survivor")
	network.Router.MarkConnected(gone)
	network.Router.MarkConnected(survivor)

	// One link each way, since a deleted router is an endpoint of links it dialed and links it accepted,
	// and the index is keyed on both.
	dialed := model.NewTestLink("dialed-by-gone", gone, survivor)
	dialed.SetState(model.Connected)
	network.Link.Add(dialed)

	accepted := model.NewTestLink("dialed-by-survivor", survivor, gone)
	accepted.SetState(model.Connected)
	network.Link.Add(accepted)

	req.Len(network.Link.LinksForRouter(gone.Id), 2)
	req.Len(network.Link.LinksForRouter(survivor.Id), 2)

	req.NoError(network.Router.Delete(gone.Id, change.New()))
	network.runRouterDeletedCleanups(gone.Id)

	_, found := network.Link.Get(dialed.Id)
	req.False(found, "a link dialed by the deleted router must be removed")
	_, found = network.Link.Get(accepted.Id)
	req.False(found, "a link accepted by the deleted router must be removed")

	req.Empty(network.Link.LinksForRouter(survivor.Id),
		"the surviving endpoint must not be left holding links to a router that is gone")
}

// TestScanForDeadLinks_RetiresLegacyLinksButNotGossipedOnes covers the distinction the scan's safety rests
// on, which is easy to lose and expensive to get wrong in either direction.
//
// A router reporting the pre-gossip way sends its whole dialed set every time it reconnects, so dropping
// its links while it is away costs nothing. A router that gossips deliberately does not re-announce,
// leaving that to anti-entropy, so dropping its links would strand them.
//
// Without the scan, a pre-gossip router that restarts leaves its entire dialed set behind: the link ids are
// minted per dial, so the router never mentions the old ones again, and nothing else retires them.
func TestScanForDeadLinks_RetiresLegacyLinksButNotGossipedOnes(t *testing.T) {
	req := require.New(t)
	_, network, addr := newConnectTestNetwork(t)

	legacy := newPersistedRouter(t, network, addr, "legacy")
	gossiping := newPersistedRouter(t, network, addr, "gossiping")
	peer := newPersistedRouter(t, network, addr, "peer")
	network.Router.MarkConnected(peer)

	legacyLink := model.NewTestLink("legacy-link", legacy, peer)
	legacyLink.SetState(model.Connected)
	network.Link.Add(legacyLink)

	gossipedLink := model.NewTestLink("gossiped-link", gossiping, peer)
	gossipedLink.SetState(model.Connected)
	network.Link.Add(gossipedLink)

	// Only the second is published in gossip, which is what marks it as one whose router will not
	// re-announce it.
	req.NoError(network.LinkGossipType.Set(
		LinkGossipKey(gossipedLink.Id, gossipedLink.Iteration), gossiping.Id,
		&ctrl_pb.RouterLinks_RouterLink{Id: gossipedLink.Id, DestRouterId: peer.Id}))

	// Both dialers go away, which is the state a restart leaves behind. NewTestLink marks its endpoints
	// connected, so this has to come after the links are built.
	legacy.Connected.Store(false)
	gossiping.Connected.Store(false)

	network.scanForDeadLinks()

	_, found := network.Link.Get(legacyLink.Id)
	req.False(found, "a link its router will re-announce must be retired while that router is away")

	_, found = network.Link.Get(gossipedLink.Id)
	req.True(found, "a gossiped link must be left to anti-entropy, since its router will not re-announce it")
}

// TestScanForDeadLinks_KeepsLinksOfConnectedRouters: the scan is about routers that are gone, not about
// links it dislikes.
func TestScanForDeadLinks_KeepsLinksOfConnectedRouters(t *testing.T) {
	req := require.New(t)
	_, network, addr := newConnectTestNetwork(t)

	src := newPersistedRouter(t, network, addr, "src")
	dst := newPersistedRouter(t, network, addr, "dst")
	network.Router.MarkConnected(src)
	network.Router.MarkConnected(dst)

	link := model.NewTestLink("live", src, dst)
	link.SetState(model.Connected)
	network.Link.Add(link)

	network.scanForDeadLinks()

	_, found := network.Link.Get(link.Id)
	req.True(found, "a connected router's links must be left alone")
}
