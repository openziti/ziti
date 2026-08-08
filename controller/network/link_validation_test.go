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

	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/ziti/v2/common/inspect"
	"github.com/openziti/ziti/v2/common/pb/mgmt_pb"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/stretchr/testify/require"
)

// newDbLoadedSrcLink builds a link whose source is a router instance loaded from the database. Such an
// instance carries no version, since a router's version arrives in its hello and is not persisted.
func newDbLoadedSrcLink(t *testing.T, srcId string) *model.Link {
	t.Helper()
	src := &model.Router{}
	src.Id = srcId
	require.Nil(t, src.VersionInfo, "a database-loaded router carries no version")

	return &model.Link{Id: "l1", DstId: "dst", Src: src}
}

// TestCheckLinkConns_UnknownVersionIsSkipped: with no connected instance there is no way to know what the
// router reports, so the comparison is skipped rather than failed, matching how a router too old to report
// conn info is treated.
func TestCheckLinkConns_UnknownVersionIsSkipped(t *testing.T) {
	_, network, _ := newConnectTestNetwork(t)

	link := newDbLoadedSrcLink(t, "r1")

	result := &mgmt_pb.RouterLinkDetail{IsValid: true}
	network.checkLinkConns(link, &inspect.LinkInspectDetail{}, result)

	require.True(t, result.IsValid, "an unknown router version must not make a link invalid")
	require.Empty(t, result.Messages)
}

// TestCheckLinkConns_UsesConnectedVersion: the version lives on the connected instance, so reading it off
// whatever instance the link references reports a link as invalid whenever that instance is one loaded from
// the database rather than the connected one.
func TestCheckLinkConns_UsesConnectedVersion(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	link := newDbLoadedSrcLink(t, "r1")

	connected := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	connected.VersionInfo = &versions.VersionInfo{Version: "v1.0.0"}
	network.Router.MarkConnected(connected)

	result := &mgmt_pb.RouterLinkDetail{IsValid: true}
	network.checkLinkConns(link, &inspect.LinkInspectDetail{}, result)

	require.True(t, result.IsValid, "the connected instance's version must be what decides the comparison")
	require.Empty(t, result.Messages)
}

// TestCheckLinkConns_ComparesWhenVersionKnown: once the version is known and high enough, the conn info
// comparison proceeds, so skipping above does not quietly disable the check.
func TestCheckLinkConns_ComparesWhenVersionKnown(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	connected := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	connected.VersionInfo = &versions.VersionInfo{Version: "v1.6.6"}
	network.Router.MarkConnected(connected)

	link := &model.Link{Id: "l1", DstId: "dst", Src: connected}

	// The router reports a connection the controller does not know about, which the comparison must catch.
	routerLink := &inspect.LinkInspectDetail{
		Connections: []*inspect.LinkConnection{
			{Type: "default", Source: "127.0.0.1:1000", Dest: "127.0.0.1:2000"},
		},
	}

	result := &mgmt_pb.RouterLinkDetail{IsValid: true}
	network.checkLinkConns(link, routerLink, result)

	// A conn count mismatch reports a message without marking the link invalid, so the message is what
	// shows the comparison ran rather than being skipped.
	require.NotEmpty(t, result.Messages, "a known version must let the conn info comparison run")
	require.Contains(t, result.Messages[0], "len(ctrlConns)")
}
