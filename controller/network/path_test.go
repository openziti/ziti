/*
	Copyright NetFoundry, Inc.

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
	"time"

	"github.com/stretchr/testify/require"

	"github.com/openziti/channel"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/transport/v2"
	"github.com/openziti/transport/v2/tcp"
	"github.com/stretchr/testify/assert"
)

func TestSimplePath2(t *testing.T) {
	ctx := db.NewTestContext(t)
	defer ctx.Cleanup()

	config := newTestConfig(ctx)
	defer close(config.closeNotify)

	network, err := NewNetwork(config)
	assert.Nil(t, err)

	addr := "tcp:0.0.0.0:0"
	transportAddr, err := tcp.AddressParser{}.Parse(addr)
	assert.Nil(t, err)

	r0 := newRouterForTest("r0", "", transportAddr, nil, 0, false)
	network.Routers.markConnected(r0)

	r1 := newRouterForTest("r1", "", transportAddr, nil, 0, false)
	network.Routers.markConnected(r1)

	l0 := newTestLink("l0", "tls")
	l0.Src = r0
	l0.Dst = r1
	l0.addState(newLinkState(Connected))
	network.linkController.add(l0)

	path, err := network.CreatePath(r0, r1)
	assert.NotNil(t, path)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(path.Nodes))
	assert.Equal(t, r0, path.Nodes[0])
	assert.Equal(t, r1, path.Nodes[1])
	assert.Equal(t, 1, len(path.Links))
	assert.Equal(t, l0, path.Links[0])
	assert.Equal(t, r1, path.EgressRouter())

	terminator := &Terminator{Address: addr, Binding: "transport"}
	routeMessages := path.CreateRouteMessages(0, "s0", terminator, time.Now().Add(DefaultNetworkOptionsRouteTimeout))
	assert.NotNil(t, routeMessages)
	assert.Equal(t, 2, len(routeMessages))

	// ingress route message
	rm0 := routeMessages[0]
	assert.Equal(t, "s0", rm0.CircuitId)
	assert.Nil(t, rm0.Egress)
	assert.Equal(t, 2, len(rm0.Forwards))
	assert.Equal(t, path.IngressId, rm0.Forwards[0].SrcAddress)
	assert.Equal(t, l0.Id, rm0.Forwards[0].DstAddress)
	assert.Equal(t, l0.Id, rm0.Forwards[1].SrcAddress)
	assert.Equal(t, path.IngressId, rm0.Forwards[1].DstAddress)

	// egress route message
	rm1 := routeMessages[1]
	assert.Equal(t, "s0", rm1.CircuitId)
	assert.NotNil(t, rm1.Egress)
	assert.Equal(t, path.EgressId, rm1.Egress.Address)
	assert.Equal(t, addr, rm1.Egress.Destination)
	assert.Equal(t, path.EgressId, rm1.Forwards[0].SrcAddress)
	assert.Equal(t, l0.Id, rm1.Forwards[0].DstAddress)
	assert.Equal(t, l0.Id, rm1.Forwards[1].SrcAddress)
	assert.Equal(t, path.EgressId, rm1.Forwards[1].DstAddress)
}

func TestTransitPath2(t *testing.T) {
	ctx := db.NewTestContext(t)
	defer ctx.Cleanup()

	config := newTestConfig(ctx)
	defer close(config.closeNotify)

	network, err := NewNetwork(config)
	assert.Nil(t, err)

	addr := "tcp:0.0.0.0:0"
	transportAddr, err := tcp.AddressParser{}.Parse(addr)
	assert.Nil(t, err)

	r0 := newRouterForTest("r0", "", transportAddr, nil, 0, false)
	network.Routers.markConnected(r0)

	r1 := newRouterForTest("r1", "", transportAddr, nil, 0, false)
	network.Routers.markConnected(r1)

	r2 := newRouterForTest("r2", "", transportAddr, nil, 0, false)
	network.Routers.markConnected(r2)

	l0 := newTestLink("l0", "tls")
	l0.Src = r0
	l0.Dst = r1
	l0.addState(newLinkState(Connected))
	network.linkController.add(l0)

	l1 := newTestLink("l1", "tls")
	l1.Src = r1
	l1.Dst = r2
	l1.addState(newLinkState(Connected))
	network.linkController.add(l1)

	path, err := network.CreatePath(r0, r2)
	assert.NotNil(t, path)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(path.Nodes))
	assert.Equal(t, r0, path.Nodes[0])
	assert.Equal(t, r1, path.Nodes[1])
	assert.Equal(t, r2, path.Nodes[2])
	assert.Equal(t, 2, len(path.Links))
	assert.Equal(t, l0, path.Links[0])
	assert.Equal(t, l1, path.Links[1])
	assert.Equal(t, r2, path.EgressRouter())

	terminator := &Terminator{Address: addr, Binding: "transport"}
	routeMessages := path.CreateRouteMessages(0, "s0", terminator, time.Now().Add(DefaultNetworkOptionsRouteTimeout))
	assert.NotNil(t, routeMessages)
	assert.Equal(t, 3, len(routeMessages))

	// ingress route message
	rm0 := routeMessages[0]
	assert.Equal(t, "s0", rm0.CircuitId)
	assert.Nil(t, rm0.Egress)
	assert.Equal(t, 2, len(rm0.Forwards))
	assert.Equal(t, path.IngressId, rm0.Forwards[0].SrcAddress)
	assert.Equal(t, l0.Id, rm0.Forwards[0].DstAddress)
	assert.Equal(t, l0.Id, rm0.Forwards[1].SrcAddress)
	assert.Equal(t, path.IngressId, rm0.Forwards[1].DstAddress)

	// transit route message
	rm1 := routeMessages[1]
	assert.Equal(t, "s0", rm1.CircuitId)
	assert.Nil(t, rm1.Egress)
	assert.Equal(t, 2, len(rm1.Forwards))
	assert.Equal(t, l0.Id, rm1.Forwards[0].SrcAddress)
	assert.Equal(t, l1.Id, rm1.Forwards[0].DstAddress)
	assert.Equal(t, l1.Id, rm1.Forwards[1].SrcAddress)
	assert.Equal(t, l0.Id, rm1.Forwards[1].DstAddress)

	// egress route message
	rm2 := routeMessages[2]
	assert.Equal(t, "s0", rm2.CircuitId)
	assert.NotNil(t, rm2.Egress)
	assert.Equal(t, path.EgressId, rm2.Egress.Address)
	assert.Equal(t, transportAddr.String(), rm2.Egress.Destination)
	assert.Equal(t, path.EgressId, rm2.Forwards[0].SrcAddress)
	assert.Equal(t, l1.Id, rm2.Forwards[0].DstAddress)
	assert.Equal(t, l1.Id, rm2.Forwards[1].SrcAddress)
	assert.Equal(t, path.EgressId, rm2.Forwards[1].DstAddress)
}

func newRouterForTest(id string, fingerprint string, advLstnr transport.Address, ctrl channel.Channel, cost uint16, noTraversal bool) *Router {
	r := &Router{
		BaseEntity:  models.BaseEntity{Id: id},
		Name:        id,
		Fingerprint: &fingerprint,
		Control:     ctrl,
		Cost:        cost,
		NoTraversal: noTraversal,
	}
	if advLstnr != nil {
		r.AddLinkListener(advLstnr.String(), advLstnr.Type(), []string{"Cost Tag"})
	}
	return r
}

func TestShortestPath(t *testing.T) {
	ctx := db.NewTestContext(t)
	defer ctx.Cleanup()

	req := assert.New(t)

	config := newTestConfig(ctx)
	defer close(config.closeNotify)

	network, err := NewNetwork(config)
	req.NoError(err)

	addr := "tcp:0.0.0.0:0"
	transportAddr, err := tcp.AddressParser{}.Parse(addr)
	req.NoError(err)

	r0 := newRouterForTest("r0", "", transportAddr, nil, 1, false)
	network.Routers.markConnected(r0)

	r1 := newRouterForTest("r1", "", transportAddr, nil, 2, false)
	network.Routers.markConnected(r1)

	r2 := newRouterForTest("r2", "", transportAddr, nil, 3, false)
	network.Routers.markConnected(r2)

	r3 := newRouterForTest("r3", "", transportAddr, nil, 4, false)
	network.Routers.markConnected(r3)

	link := newTestLink("l0", "tls")
	link.SetStaticCost(2)
	link.SetDstLatency(10 * 1_000_000)
	link.SetSrcLatency(11 * 1_000_000)
	link.Src = r0
	link.Dst = r1
	link.addState(newLinkState(Connected))
	network.linkController.add(link)

	link = newTestLink("l1", "tls")
	link.SetStaticCost(5)
	link.SetDstLatency(15 * 1_000_000)
	link.SetSrcLatency(16 * 1_000_000)
	link.Src = r0
	link.Dst = r2
	link.addState(newLinkState(Connected))
	network.linkController.add(link)

	link = newTestLink("l2", "tls")
	link.SetStaticCost(9)
	link.SetDstLatency(20 * 1_000_000)
	link.SetSrcLatency(21 * 1_000_000)
	link.Src = r1
	link.Dst = r3
	link.addState(newLinkState(Connected))
	network.linkController.add(link)

	link = newTestLink("l3", "tls")
	link.SetStaticCost(13)
	link.SetDstLatency(25 * 1_000_000)
	link.SetSrcLatency(26 * 1_000_000)
	link.Src = r2
	link.Dst = r3
	link.addState(newLinkState(Connected))
	network.linkController.add(link)

	path, cost, err := network.shortestPath(r0, r3)
	req.NoError(err)
	req.NotNil(t, path)
	req.Equal(path[0], r0)
	req.Equal(path[1], r1)
	req.Equal(path[2], r3)

	expected := 10 + 11 + 2 + 2 + // link1 cost and src and dest latency plus dest router cost
		9 + 20 + 21 + 4 // link2 cost and src and dest latency plus dest router cost
	req.Equal(int64(expected), cost)
}

func TestShortestPathWithUntraversableRouter(t *testing.T) {
	ctx := db.NewTestContext(t)
	defer ctx.Cleanup()

	req := assert.New(t)

	config := newTestConfig(ctx)
	defer close(config.closeNotify)

	network, err := NewNetwork(config)
	req.NoError(err)

	addr := "tcp:0.0.0.0:0"
	transportAddr, err := tcp.AddressParser{}.Parse(addr)
	req.NoError(err)

	r0 := newRouterForTest("r0", "", transportAddr, nil, 1, false)
	network.Routers.markConnected(r0)

	r1 := newRouterForTest("r1", "", transportAddr, nil, 2, true)
	network.Routers.markConnected(r1)

	r2 := newRouterForTest("r2", "", transportAddr, nil, 3, false)
	network.Routers.markConnected(r2)

	r3 := newRouterForTest("r3", "", transportAddr, nil, 4, false)
	network.Routers.markConnected(r3)

	link := newTestLink("l0", "tls")
	link.SetStaticCost(2)
	link.SetDstLatency(10 * 1_000_000)
	link.SetSrcLatency(11 * 1_000_000)
	link.Src = r0
	link.Dst = r1
	link.addState(newLinkState(Connected))
	network.linkController.add(link)

	link = newTestLink("l1", "tls")
	link.SetStaticCost(5)
	link.SetDstLatency(15 * 1_000_000)
	link.SetSrcLatency(16 * 1_000_000)
	link.Src = r0
	link.Dst = r2
	link.addState(newLinkState(Connected))
	network.linkController.add(link)

	link = newTestLink("l2", "tls")
	link.SetStaticCost(9)
	link.SetDstLatency(20 * 1_000_000)
	link.SetSrcLatency(21 * 1_000_000)
	link.Src = r1
	link.Dst = r3
	link.addState(newLinkState(Connected))
	network.linkController.add(link)

	link = newTestLink("l3", "tls")
	link.SetStaticCost(13)
	link.SetDstLatency(25 * 1_000_000)
	link.SetSrcLatency(26 * 1_000_000)
	link.Src = r2
	link.Dst = r3
	link.addState(newLinkState(Connected))
	network.linkController.add(link)

	path, cost, err := network.shortestPath(r0, r3)
	req.NoError(err)
	req.NotNil(t, path)
	req.Equal(path[0], r0)
	req.Equal(path[1], r2)
	req.Equal(path[2], r3)

	expected := 15 + 16 + 5 + 3 + // link1 cost and src and dest latency plus dest router cost
		25 + 26 + 13 + 4 // link3 cost and src and dest latency plus dest router cost
	req.Equal(int64(expected), cost)
}

func TestShortestPathWithOnlyUntraversableRouter(t *testing.T) {
	ctx := db.NewTestContext(t)
	defer ctx.Cleanup()

	req := assert.New(t)

	config := newTestConfig(ctx)
	defer close(config.closeNotify)

	network, err := NewNetwork(config)
	req.NoError(err)

	addr := "tcp:0.0.0.0:0"
	transportAddr, err := tcp.AddressParser{}.Parse(addr)
	req.NoError(err)

	r0 := newRouterForTest("r0", "", transportAddr, nil, 1, false)
	network.Routers.markConnected(r0)

	r1 := newRouterForTest("r1", "", transportAddr, nil, 2, true)
	network.Routers.markConnected(r1)

	link := newTestLink("l0", "tls")
	link.SetStaticCost(2)
	link.SetDstLatency(10 * 1_000_000)
	link.SetSrcLatency(11 * 1_000_000)
	link.Src = r0
	link.Dst = r1
	link.addState(newLinkState(Connected))
	network.linkController.add(link)

	path, cost, err := network.shortestPath(r0, r1)
	req.NoError(err)
	req.NotNil(t, path)
	req.Equal(path[0], r0)
	req.Equal(path[1], r1)

	expected := 2 + 10 + 11 + 2 // link0 cost and src and dest latency plus dest router cost

	req.Equal(int64(expected), cost)
}

func TestShortestPathWithUntraversableEdgeRouters(t *testing.T) {
	ctx := db.NewTestContext(t)
	defer ctx.Cleanup()

	req := assert.New(t)

	config := newTestConfig(ctx)
	defer close(config.closeNotify)

	network, err := NewNetwork(config)
	req.NoError(err)

	addr := "tcp:0.0.0.0:0"
	transportAddr, err := tcp.AddressParser{}.Parse(addr)
	req.NoError(err)

	r0 := newRouterForTest("r0", "", transportAddr, nil, 1, true)
	network.Routers.markConnected(r0)

	r1 := newRouterForTest("r1", "", transportAddr, nil, 2, true)
	network.Routers.markConnected(r1)

	link := newTestLink("l0", "tls")
	link.SetStaticCost(3)
	link.SetDstLatency(10 * 1_000_000)
	link.SetSrcLatency(11 * 1_000_000)
	link.Src = r0
	link.Dst = r1
	link.addState(newLinkState(Connected))
	network.linkController.add(link)

	path, cost, err := network.shortestPath(r0, r1)
	req.NoError(err)
	req.NotNil(t, path)
	req.Equal(path[0], r0)
	req.Equal(path[1], r1)

	expected := 3 + 10 + 11 + 2 // link0 cost and src and dest latency plus dest router cost

	req.Equal(int64(expected), cost)
}

func TestShortestPathWithUntraversableEdgeRoutersAndTraversableMiddle(t *testing.T) {
	ctx := db.NewTestContext(t)
	defer ctx.Cleanup()

	req := assert.New(t)

	config := newTestConfig(ctx)
	defer close(config.closeNotify)

	network, err := NewNetwork(config)
	req.NoError(err)

	addr := "tcp:0.0.0.0:0"
	transportAddr, err := tcp.AddressParser{}.Parse(addr)
	req.NoError(err)

	r0 := newRouterForTest("r0", "", transportAddr, nil, 1, true)
	network.Routers.markConnected(r0)

	r1 := newRouterForTest("r1", "", transportAddr, nil, 2, false)
	network.Routers.markConnected(r1)

	r2 := newRouterForTest("r2", "", transportAddr, nil, 3, true)
	network.Routers.markConnected(r2)

	link := newTestLink("l0", "tls")
	link.SetStaticCost(2)
	link.SetDstLatency(10 * 1_000_000)
	link.SetSrcLatency(11 * 1_000_000)
	link.Src = r0
	link.Dst = r1
	link.addState(newLinkState(Connected))
	network.linkController.add(link)

	link = newTestLink("l1", "tls")
	link.SetStaticCost(3)
	link.SetDstLatency(12 * 1_000_000)
	link.SetSrcLatency(15 * 1_000_000)
	link.Src = r1
	link.Dst = r2
	link.addState(newLinkState(Connected))
	network.linkController.add(link)

	path, cost, err := network.shortestPath(r0, r2)
	req.NoError(err)
	req.NotNil(t, path)
	req.Equal(path[0], r0)
	req.Equal(path[1], r1)
	req.Equal(path[2], r2)

	expected := 2 + 10 + 11 + 2 + // link0 cost and src and dest latency plus dest router cost
		3 + 12 + 15 + 3 // link1 cost and src and dest latency plus dest router cost

	req.Equal(int64(expected), cost)
}

func TestShortestPathWithUntraversableEdgeRoutersAndUntraversableMiddle(t *testing.T) {
	ctx := db.NewTestContext(t)
	defer ctx.Cleanup()

	req := assert.New(t)

	config := newTestConfig(ctx)
	defer close(config.closeNotify)

	network, err := NewNetwork(config)
	req.NoError(err)

	addr := "tcp:0.0.0.0:0"
	transportAddr, err := tcp.AddressParser{}.Parse(addr)
	req.NoError(err)

	r0 := newRouterForTest("r0", "", transportAddr, nil, 1, true)
	network.Routers.markConnected(r0)

	r1 := newRouterForTest("r1", "", transportAddr, nil, 2, true)
	network.Routers.markConnected(r1)

	r2 := newRouterForTest("r2", "", transportAddr, nil, 2, true)
	network.Routers.markConnected(r2)

	link := newTestLink("l0", "tls")
	link.SetStaticCost(2)
	link.SetDstLatency(10 * 1_000_000)
	link.SetSrcLatency(11 * 1_000_000)
	link.Src = r0
	link.Dst = r1
	link.addState(newLinkState(Connected))
	network.linkController.add(link)

	link = newTestLink("l2", "tls")
	link.SetStaticCost(2)
	link.SetDstLatency(10 * 1_000_000)
	link.SetSrcLatency(11 * 1_000_000)
	link.Src = r1
	link.Dst = r2
	link.addState(newLinkState(Connected))
	network.linkController.add(link)

	path, cost, err := network.shortestPath(r0, r2)
	req.Error(err)
	req.NotNil(t, path)
	req.Len(path, 0)

	req.Equal(int64(0), cost)
}

func TestRouterCost(t *testing.T) {
	ctx := db.NewTestContext(t)
	defer ctx.Cleanup()

	req := require.New(t)

	config := newTestConfig(ctx)
	defer close(config.closeNotify)

	network, err := NewNetwork(config)
	req.NoError(err)

	addr := "tcp:0.0.0.0:0"
	transportAddr, err := tcp.AddressParser{}.Parse(addr)
	req.NoError(err)

	r0 := newRouterForTest("r0", "", transportAddr, nil, 10, true)
	network.Routers.markConnected(r0)

	r1 := newRouterForTest("r1", "", transportAddr, nil, 100, false)
	network.Routers.markConnected(r1)

	r2 := newRouterForTest("r2", "", transportAddr, nil, 200, false)
	network.Routers.markConnected(r2)

	r3 := newRouterForTest("r3", "", transportAddr, nil, 20, true)
	network.Routers.markConnected(r3)

	newPathTestLink(network, "l0", r0, r1)
	newPathTestLink(network, "l1", r0, r2)
	newPathTestLink(network, "l2", r1, r3)
	newPathTestLink(network, "l3", r2, r3)

	path, cost, err := network.shortestPath(r0, r3)
	req.NoError(err)
	req.NotNil(t, path)
	req.Len(path, 3)
	req.Equal("r0", path[0].Id)
	req.Equal("r1", path[1].Id)
	req.Equal("r3", path[2].Id)

	req.Equal(int64(122), cost)

	r1.Cost = 300

	path, cost, err = network.shortestPath(r0, r3)
	req.NoError(err)
	req.NotNil(t, path)
	req.Len(path, 3)
	req.Equal("r0", path[0].Id)
	req.Equal("r2", path[1].Id)
	req.Equal("r3", path[2].Id)

	req.Equal(int64(222), cost)
}

func TestMinRouterCost(t *testing.T) {
	ctx := db.NewTestContext(t)
	defer ctx.Cleanup()

	req := require.New(t)

	config := newTestConfig(ctx)
	defer close(config.closeNotify)

	config.options.MinRouterCost = 10
	network, err := NewNetwork(config)
	req.NoError(err)

	addr := "tcp:0.0.0.0:0"
	transportAddr, err := tcp.AddressParser{}.Parse(addr)
	req.NoError(err)

	r0 := newRouterForTest("r0", "", transportAddr, nil, 0, true)
	network.Routers.markConnected(r0)

	r1 := newRouterForTest("r1", "", transportAddr, nil, 7, false)
	network.Routers.markConnected(r1)

	r2 := newRouterForTest("r2", "", transportAddr, nil, 200, false)
	network.Routers.markConnected(r2)

	r3 := newRouterForTest("r3", "", transportAddr, nil, 20, true)
	network.Routers.markConnected(r3)

	newPathTestLink(network, "l0", r0, r1)
	newPathTestLink(network, "l1", r0, r2)
	newPathTestLink(network, "l2", r1, r3)
	newPathTestLink(network, "l3", r2, r3)

	path, cost, err := network.shortestPath(r0, r3)
	req.NoError(err)
	req.NotNil(t, path)
	req.Len(path, 3)
	req.Equal("r0", path[0].Id)
	req.Equal("r1", path[1].Id)
	req.Equal("r3", path[2].Id)

	req.Equal(int64(32), cost)

	r1.Cost = 300

	path, cost, err = network.shortestPath(r0, r3)
	req.NoError(err)
	req.NotNil(t, path)
	req.Len(path, 3)
	req.Equal("r0", path[0].Id)
	req.Equal("r2", path[1].Id)
	req.Equal("r3", path[2].Id)

	req.Equal(int64(222), cost)
}

func newPathTestLink(network *Network, id string, srcR, destR *Router) *Link {
	l := newTestLink(id, "tls")
	l.Src = srcR
	l.Dst = destR
	l.SrcLatency = 0
	l.DstLatency = 0
	l.recalculateCost()
	l.addState(newLinkState(Connected))
	network.linkController.add(l)
	return l
}
