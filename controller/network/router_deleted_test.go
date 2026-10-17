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
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/stretchr/testify/require"
)

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
	network.retireLinksForDeletedRouter(gone.Id)

	_, found := network.Link.Get(dialed.Id)
	req.False(found, "a link dialed by the deleted router must be removed")
	_, found = network.Link.Get(accepted.Id)
	req.False(found, "a link accepted by the deleted router must be removed")

	req.Empty(network.Link.LinksForRouter(survivor.Id),
		"the surviving endpoint must not be left holding links to a router that is gone")
}

// TestScanForDeadLinks_DialerReturningMidScanKeepsItsLinks is the race that cost a soak run 53 links. The
// scan walks the link table and then removes from it in a second pass, and on a large table that gap is
// wide enough for a dialer to reconnect and re-announce. Removing on the first pass's answer drops a link
// the controller was just told about, and a router reporting the pre-gossip way says its set only on
// reconnect, so it never mentions that link again.
//
// The keep callback stands in for the reconnect, since it runs between the scan's test of the dialer and
// the removal that acts on it.
func TestScanForDeadLinks_DialerReturningMidScanKeepsItsLinks(t *testing.T) {
	req := require.New(t)
	_, network, addr := newConnectTestNetwork(t)

	src := newPersistedRouter(t, network, addr, "returning")
	peer := newPersistedRouter(t, network, addr, "peer")
	network.Router.MarkConnected(peer)

	link := model.NewTestLink("reannounced", src, peer)
	link.SetState(model.Connected)
	network.Link.Add(link)

	// Away when the scan starts, which is what puts the link on the list.
	src.Connected.Store(false)

	network.Link.ScanForDeadLinks(func(*model.Link) bool {
		src.Connected.Store(true)
		return false
	})

	_, found := network.Link.Get(link.Id)
	req.True(found, "a link whose dialer returned before the removal must be kept, since the dialer has "+
		"already re-announced it and will not do so again until it next connects")
}

// TestScanForDeadLinks_StillRetiresADialerThatStaysAway guards the other direction: re-testing must not turn
// the scan into a no-op, or a restarted router's abandoned links accumulate forever.
func TestScanForDeadLinks_StillRetiresADialerThatStaysAway(t *testing.T) {
	req := require.New(t)
	_, network, addr := newConnectTestNetwork(t)

	src := newPersistedRouter(t, network, addr, "gone")
	peer := newPersistedRouter(t, network, addr, "peer")
	network.Router.MarkConnected(peer)

	link := model.NewTestLink("abandoned", src, peer)
	link.SetState(model.Connected)
	network.Link.Add(link)
	src.Connected.Store(false)

	network.Link.ScanForDeadLinks(nil)

	_, found := network.Link.Get(link.Id)
	req.False(found, "a dialer that is still away must have its links retired")
}
