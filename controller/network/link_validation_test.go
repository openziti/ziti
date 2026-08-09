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
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/v2/common/inspect"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/common/pb/mgmt_pb"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/stretchr/testify/require"
)

// newPlaceholderSrcLink builds a link whose source is the database-loaded placeholder a gossiped entry
// holds until the source router connects here and the link is repointed. The placeholder carries no
// version, since a router's version arrives in its hello.
func newPlaceholderSrcLink(t *testing.T, srcId string) *model.Link {
	t.Helper()
	placeholder := &model.Router{}
	placeholder.Id = srcId
	require.Nil(t, placeholder.VersionInfo, "a database-loaded router carries no version")

	link := &model.Link{Id: "l1", DstId: "dst"}
	link.Src.Store(placeholder)
	return link
}

// TestCheckLinkConns_PlaceholderSrcUsesConnectedVersion: the conn-info comparison applies only to routers
// new enough to report it, and that is decided by the router's version. The version lives on the connected
// instance, so reading it off the link's endpoint reports every link of a router whose link still holds the
// gossip placeholder as invalid, until the router happens to reconnect and the link is repointed.
func TestCheckLinkConns_PlaceholderSrcUsesConnectedVersion(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	link := newPlaceholderSrcLink(t, "r1")

	connected := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	connected.VersionInfo = &versions.VersionInfo{Version: "v1.0.0"}
	network.Router.MarkConnected(connected)

	result := &mgmt_pb.RouterLinkDetail{IsValid: true}
	network.checkLinkConns(link, &inspect.LinkInspectDetail{}, result)

	require.True(t, result.IsValid, "a link whose source is the gossip placeholder must not be reported invalid")
	require.Empty(t, result.Messages)
}

// TestCheckLinkConns_UnknownVersionIsSkipped: with no connected instance there is no way to know what the
// router reports, so the comparison is skipped rather than failed, matching how a router too old to report
// conn info is treated.
func TestCheckLinkConns_UnknownVersionIsSkipped(t *testing.T) {
	_, network, _ := newConnectTestNetwork(t)

	link := newPlaceholderSrcLink(t, "r1")

	result := &mgmt_pb.RouterLinkDetail{IsValid: true}
	network.checkLinkConns(link, &inspect.LinkInspectDetail{}, result)

	require.True(t, result.IsValid, "an unknown router version must not make a link invalid")
	require.Empty(t, result.Messages)
}

// newComparableLink builds a link whose source is connected and new enough to report conn info, so
// checkLinkConns reaches the conn comparison.
func newComparableLink(t *testing.T, network *Network, addr transport.Address) *model.Link {
	t.Helper()
	connected := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	connected.VersionInfo = &versions.VersionInfo{Version: "v1.6.6"}
	network.Router.MarkConnected(connected)

	link := &model.Link{Id: "l1", DstId: "dst"}
	link.Src.Store(connected)
	return link
}

func linkConn(connType, local, remote string) *ctrl_pb.LinkConn {
	return &ctrl_pb.LinkConn{Type: connType, LocalAddr: local, RemoteAddr: remote}
}

func routerConn(connType, local, remote string) *inspect.LinkConnection {
	return &inspect.LinkConnection{Type: connType, Source: local, Dest: remote}
}

// TestCheckLinkConns_DifferentIterationIsNotComparable: the router bumps its iteration on every underlay
// change and the controller only learns of it afterwards, so the router running ahead is the ordinary state
// of an update in flight. The conns are expected to differ until it lands, and reporting that as a
// disagreement would fail links for being momentarily behind.
func TestCheckLinkConns_DifferentIterationIsNotComparable(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	link := newComparableLink(t, network, addr)
	link.SetConnsState(&ctrl_pb.LinkConnState{
		StateIteration: 4,
		Conns:          []*ctrl_pb.LinkConn{linkConn("default", "127.0.0.1:1000", "127.0.0.1:2000")},
	})

	// The router has since added an underlay and is reporting the newer state.
	routerLink := &inspect.LinkInspectDetail{
		ConnStateIteration: 5,
		Connections: []*inspect.LinkConnection{
			routerConn("default", "127.0.0.1:1000", "127.0.0.1:2000"),
			routerConn("high", "127.0.0.1:1001", "127.0.0.1:2001"),
		},
	}

	result := &mgmt_pb.RouterLinkDetail{IsValid: true}
	network.checkLinkConns(link, routerLink, result)

	require.True(t, result.IsValid, "views from different iterations must not fail the link")
	require.Empty(t, result.Messages, "views from different iterations have nothing to report")
}

// TestCheckLinkConns_SameIterationCountMismatchIsInvalid: at the same iteration both sides describe the
// same state, so a differing conn count is a real disagreement rather than one view being behind the
// other. Multi-underlay links make this the likeliest disagreement, and a count check that treats it as
// valid reports nothing at all.
func TestCheckLinkConns_SameIterationCountMismatchIsInvalid(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	link := newComparableLink(t, network, addr)
	link.SetConnsState(&ctrl_pb.LinkConnState{
		StateIteration: 5,
		Conns:          []*ctrl_pb.LinkConn{linkConn("default", "127.0.0.1:1000", "127.0.0.1:2000")},
	})

	routerLink := &inspect.LinkInspectDetail{
		ConnStateIteration: 5,
		Connections: []*inspect.LinkConnection{
			routerConn("default", "127.0.0.1:1000", "127.0.0.1:2000"),
			routerConn("high", "127.0.0.1:1001", "127.0.0.1:2001"),
		},
	}

	result := &mgmt_pb.RouterLinkDetail{IsValid: true}
	network.checkLinkConns(link, routerLink, result)

	require.False(t, result.IsValid, "a conn count disagreement at the same iteration must fail the link")
	require.Len(t, result.Messages, 1)
	require.Contains(t, result.Messages[0], "len(ctrlConns): 1 != len(routerConns): 2")
}

// TestCheckLinkConns_SameIterationAgreementIsValid: agreeing views must stay valid, so the checks above do
// not fail every link with conns.
func TestCheckLinkConns_SameIterationAgreementIsValid(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	link := newComparableLink(t, network, addr)
	link.SetConnsState(&ctrl_pb.LinkConnState{
		StateIteration: 5,
		Conns: []*ctrl_pb.LinkConn{
			linkConn("high", "127.0.0.1:1001", "127.0.0.1:2001"),
			linkConn("default", "127.0.0.1:1000", "127.0.0.1:2000"),
		},
	})

	// Reported in a different order, which the comparison sorts out before comparing.
	routerLink := &inspect.LinkInspectDetail{
		ConnStateIteration: 5,
		Connections: []*inspect.LinkConnection{
			routerConn("default", "127.0.0.1:1000", "127.0.0.1:2000"),
			routerConn("high", "127.0.0.1:1001", "127.0.0.1:2001"),
		},
	}

	result := &mgmt_pb.RouterLinkDetail{IsValid: true}
	network.checkLinkConns(link, routerLink, result)

	require.True(t, result.IsValid)
	require.Empty(t, result.Messages)
}

// TestCheckLinkConns_SameIterationAddrMismatchIsInvalid keeps the per-conn comparison covered alongside the
// count one, since both now decide validity.
func TestCheckLinkConns_SameIterationAddrMismatchIsInvalid(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	link := newComparableLink(t, network, addr)
	link.SetConnsState(&ctrl_pb.LinkConnState{
		StateIteration: 5,
		Conns:          []*ctrl_pb.LinkConn{linkConn("default", "127.0.0.1:1000", "127.0.0.1:2000")},
	})

	routerLink := &inspect.LinkInspectDetail{
		ConnStateIteration: 5,
		Connections:        []*inspect.LinkConnection{routerConn("default", "127.0.0.1:9999", "127.0.0.1:2000")},
	}

	result := &mgmt_pb.RouterLinkDetail{IsValid: true}
	network.checkLinkConns(link, routerLink, result)

	require.False(t, result.IsValid)
	require.Len(t, result.Messages, 1)
	require.Contains(t, result.Messages[0], "local addr doesn't match")
}

// TestCheckLinkDest covers the only thing that says a link is not carrying adjacency. A link that names a
// connected destination router without referencing it is skipped by path computation, and every other view
// reports it healthy: both ends call it established, the destination reads as connected because that is
// answered from the connected router map rather than from the link, and the link listing renders the
// destination by reading it back from the database.
func TestCheckLinkDest(t *testing.T) {
	newLink := func(t *testing.T, withDest bool) *model.Link {
		t.Helper()
		src := &model.Router{}
		src.Id = "r0"
		dst := &model.Router{}
		dst.Id = "r1"

		link := &model.Link{Id: "l0", DstId: dst.Id}
		link.Src.Store(src)
		if withDest {
			link.Dst.Store(dst)
		} else {
			link.Dst.Store((*model.Router)(nil))
		}
		return link
	}

	t.Run("connected destination that the link does not reference", func(t *testing.T) {
		detail := &mgmt_pb.RouterLinkDetail{DestConnected: true, IsValid: true}
		checkLinkDest(newLink(t, false), detail)

		require.False(t, detail.IsValid, "a link carrying no adjacency must not validate")
		require.Len(t, detail.Messages, 1)
	})

	t.Run("connected destination that the link references", func(t *testing.T) {
		detail := &mgmt_pb.RouterLinkDetail{DestConnected: true, IsValid: true}
		checkLinkDest(newLink(t, true), detail)

		require.True(t, detail.IsValid)
		require.Empty(t, detail.Messages)
	})

	// Routine rather than a fault: in HA the far end is often connected to a different controller, and a link
	// to a router this one has no connection to is expected to reference none.
	t.Run("destination that is not connected here", func(t *testing.T) {
		detail := &mgmt_pb.RouterLinkDetail{DestConnected: false, IsValid: true}
		checkLinkDest(newLink(t, false), detail)

		require.True(t, detail.IsValid)
		require.Empty(t, detail.Messages)
	})
}
