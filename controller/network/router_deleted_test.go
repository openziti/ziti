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
