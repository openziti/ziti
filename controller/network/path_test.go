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
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/common"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/transport"
	"github.com/openziti/foundation/transport/tcp"
	"github.com/stretchr/testify/assert"
	"runtime"
	"testing"
	"time"
)

func TestSimplePath2(t *testing.T) {
	ctx := db.NewTestContext(t)
	defer ctx.Cleanup()

	closeNotify := make(chan struct{})
	defer close(closeNotify)

	nodeId := &identity.TokenId{Token: "test"}
	network, err := NewNetwork(nodeId, nil, ctx.GetDb(), nil, NewVersionProviderTest(), closeNotify)
	assert.Nil(t, err)

	addr := "tcp:0.0.0.0:0"
	transportAddr, err := tcp.AddressParser{}.Parse(addr)
	assert.Nil(t, err)

	r0 := newRouterForTest("r0", "", transportAddr, nil)
	network.Routers.markConnected(r0)

	r1 := newRouterForTest("r1", "", transportAddr, nil)
	network.Routers.markConnected(r1)

	l0 := newLink(&identity.TokenId{Token: "l0"})
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

	s0 := &identity.TokenId{Token: "s0"}
	terminator := &Terminator{Address: addr, Binding: "transport"}
	routeMessages, err := path.CreateRouteMessages(0, s0, terminator)
	assert.NotNil(t, routeMessages)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(routeMessages))

	// ingress route message
	rm0 := routeMessages[0]
	assert.Equal(t, s0.Token, rm0.SessionId)
	assert.Nil(t, rm0.Egress)
	assert.Equal(t, 2, len(rm0.Forwards))
	assert.Equal(t, path.IngressId, rm0.Forwards[0].SrcAddress)
	assert.Equal(t, l0.Id.Token, rm0.Forwards[0].DstAddress)
	assert.Equal(t, l0.Id.Token, rm0.Forwards[1].SrcAddress)
	assert.Equal(t, path.IngressId, rm0.Forwards[1].DstAddress)

	// egress route message
	rm1 := routeMessages[1]
	assert.Equal(t, s0.Token, rm1.SessionId)
	assert.NotNil(t, rm1.Egress)
	assert.Equal(t, path.EgressId, rm1.Egress.Address)
	assert.Equal(t, addr, rm1.Egress.Destination)
	assert.Equal(t, path.EgressId, rm1.Forwards[0].SrcAddress)
	assert.Equal(t, l0.Id.Token, rm1.Forwards[0].DstAddress)
	assert.Equal(t, l0.Id.Token, rm1.Forwards[1].SrcAddress)
	assert.Equal(t, path.EgressId, rm1.Forwards[1].DstAddress)
}

func TestTransitPath2(t *testing.T) {
	ctx := db.NewTestContext(t)
	defer ctx.Cleanup()

	closeNotify := make(chan struct{})
	defer close(closeNotify)

	nodeId := &identity.TokenId{Token: "test"}
	network, err := NewNetwork(nodeId, nil, ctx.GetDb(), nil, NewVersionProviderTest(), closeNotify)
	assert.Nil(t, err)

	addr := "tcp:0.0.0.0:0"
	transportAddr, err := tcp.AddressParser{}.Parse(addr)
	assert.Nil(t, err)

	r0 := newRouterForTest("r0", "", transportAddr, nil)
	network.Routers.markConnected(r0)

	r1 := newRouterForTest("r1", "", transportAddr, nil)
	network.Routers.markConnected(r1)

	r2 := newRouterForTest("r2", "", transportAddr, nil)
	network.Routers.markConnected(r2)

	l0 := newLink(&identity.TokenId{Token: "l0"})
	l0.Src = r0
	l0.Dst = r1
	l0.addState(newLinkState(Connected))
	network.linkController.add(l0)

	l1 := newLink(&identity.TokenId{Token: "l1"})
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

	s0 := &identity.TokenId{Token: "s0"}
	terminator := &Terminator{Address: addr, Binding: "transport"}
	routeMessages, err := path.CreateRouteMessages(0, s0, terminator)
	assert.NotNil(t, routeMessages)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(routeMessages))

	// ingress route message
	rm0 := routeMessages[0]
	assert.Equal(t, s0.Token, rm0.SessionId)
	assert.Nil(t, rm0.Egress)
	assert.Equal(t, 2, len(rm0.Forwards))
	assert.Equal(t, path.IngressId, rm0.Forwards[0].SrcAddress)
	assert.Equal(t, l0.Id.Token, rm0.Forwards[0].DstAddress)
	assert.Equal(t, l0.Id.Token, rm0.Forwards[1].SrcAddress)
	assert.Equal(t, path.IngressId, rm0.Forwards[1].DstAddress)

	// transit route message
	rm1 := routeMessages[1]
	assert.Equal(t, s0.Token, rm1.SessionId)
	assert.Nil(t, rm1.Egress)
	assert.Equal(t, 2, len(rm1.Forwards))
	assert.Equal(t, l0.Id.Token, rm1.Forwards[0].SrcAddress)
	assert.Equal(t, l1.Id.Token, rm1.Forwards[0].DstAddress)
	assert.Equal(t, l1.Id.Token, rm1.Forwards[1].SrcAddress)
	assert.Equal(t, l0.Id.Token, rm1.Forwards[1].DstAddress)

	// egress route message
	rm2 := routeMessages[2]
	assert.Equal(t, s0.Token, rm2.SessionId)
	assert.NotNil(t, rm2.Egress)
	assert.Equal(t, path.EgressId, rm2.Egress.Address)
	assert.Equal(t, transportAddr.String(), rm2.Egress.Destination)
	assert.Equal(t, path.EgressId, rm2.Forwards[0].SrcAddress)
	assert.Equal(t, l1.Id.Token, rm2.Forwards[0].DstAddress)
	assert.Equal(t, l1.Id.Token, rm2.Forwards[1].SrcAddress)
	assert.Equal(t, path.EgressId, rm2.Forwards[1].DstAddress)
}

func newRouterForTest(id string, fingerprint string, advLstnr transport.Address, ctrl channel2.Channel) *Router {
	r := &Router{
		BaseEntity:  models.BaseEntity{Id: id},
		Fingerprint: &fingerprint,
		Control:     ctrl,
	}
	if advLstnr != nil {
		r.AdvertisedListener = advLstnr.String()
	}
	return r
}

type VersionProviderTest struct {
}

func (v VersionProviderTest) Branch() string {
	return "local"
}

func (v VersionProviderTest) EncoderDecoder() common.VersionEncDec {
	return &common.StdVersionEncDec
}

func (v VersionProviderTest) Version() string {
	return "v0.0.0"
}

func (v VersionProviderTest) BuildDate() string {
	return time.Now().String()
}

func (v VersionProviderTest) Revision() string {
	return ""
}

func (v VersionProviderTest) AsVersionInfo() *common.VersionInfo {
	return &common.VersionInfo{
		Version:   v.Version(),
		Revision:  v.Revision(),
		BuildDate: v.BuildDate(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}

func NewVersionProviderTest() common.VersionProvider {
	return &VersionProviderTest{}
}

func TestShortestPath(t *testing.T) {
	ctx := db.NewTestContext(t)
	defer ctx.Cleanup()

	req := assert.New(t)

	closeNotify := make(chan struct{})
	defer close(closeNotify)

	nodeId := &identity.TokenId{Token: "test"}
	network, err := NewNetwork(nodeId, nil, ctx.GetDb(), nil, NewVersionProviderTest(), closeNotify)
	req.NoError(err)

	addr := "tcp:0.0.0.0:0"
	transportAddr, err := tcp.AddressParser{}.Parse(addr)
	req.NoError(err)

	r0 := newRouterForTest("r0", "", transportAddr, nil)
	network.Routers.markConnected(r0)

	r1 := newRouterForTest("r1", "", transportAddr, nil)
	network.Routers.markConnected(r1)

	r2 := newRouterForTest("r2", "", transportAddr, nil)
	network.Routers.markConnected(r2)

	r3 := newRouterForTest("r3", "", transportAddr, nil)
	network.Routers.markConnected(r3)

	link := newLink(&identity.TokenId{Token: "l0"})
	link.SetStaticCost(2)
	link.SetDstLatency(10 * 1_000_000)
	link.SetSrcLatency(11 * 1_000_000)
	link.Src = r0
	link.Dst = r1
	link.addState(newLinkState(Connected))
	network.linkController.add(link)

	link = newLink(&identity.TokenId{Token: "l1"})
	link.SetStaticCost(5)
	link.SetDstLatency(15 * 1_000_000)
	link.SetSrcLatency(16 * 1_000_000)
	link.Src = r0
	link.Dst = r2
	link.addState(newLinkState(Connected))
	network.linkController.add(link)

	link = newLink(&identity.TokenId{Token: "l2"})
	link.SetStaticCost(9)
	link.SetDstLatency(20 * 1_000_000)
	link.SetSrcLatency(21 * 1_000_000)
	link.Src = r1
	link.Dst = r3
	link.addState(newLinkState(Connected))
	network.linkController.add(link)

	link = newLink(&identity.TokenId{Token: "l3"})
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

	expected := 10 + 11 + 2 + // link1 cost and src and dest latency
		9 + 20 + 21 // link2 cost and src and dest latency
	req.Equal(int64(expected), cost)
}
