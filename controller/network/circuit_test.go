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
	"github.com/netfoundry/ziti-fabric/controller/db"
	"github.com/netfoundry/ziti-foundation/identity/identity"
	"github.com/netfoundry/ziti-foundation/transport/tcp"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSimpleCircuit2(t *testing.T) {
	ctx := db.NewTestContext(t)
	defer ctx.Cleanup()

	network, err := NewNetwork(nil, nil, ctx.GetDb(), nil)
	assert.Nil(t, err)

	addr := "tcp:0.0.0.0:0"
	transportAddr, err := tcp.AddressParser{}.Parse(addr)
	assert.Nil(t, err)

	r0 := newRouter("r0", "", transportAddr, nil)
	network.Routers.markConnected(r0)

	r1 := newRouter("r1", "", transportAddr, nil)
	network.Routers.markConnected(r1)

	l0 := newLink(&identity.TokenId{Token: "l0"})
	l0.Src = r0
	l0.Dst = r1
	l0.addState(newLinkState(Connected))
	network.linkController.add(l0)

	circuit, err := network.CreateCircuit(r0, r1)
	assert.NotNil(t, circuit)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(circuit.Path))
	assert.Equal(t, r0, circuit.Path[0])
	assert.Equal(t, r1, circuit.Path[1])
	assert.Equal(t, 1, len(circuit.Links))
	assert.Equal(t, l0, circuit.Links[0])
	assert.Equal(t, r1, circuit.EgressRouter())

	s0 := &identity.TokenId{Token: "s0"}
	routeMessages, err := circuit.CreateRouteMessages(s0, addr)
	assert.NotNil(t, routeMessages)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(routeMessages))

	// ingress route message
	rm0 := routeMessages[0]
	assert.Equal(t, s0.Token, rm0.SessionId)
	assert.Nil(t, rm0.Egress)
	assert.Equal(t, 2, len(rm0.Forwards))
	assert.Equal(t, circuit.IngressId, rm0.Forwards[0].SrcAddress)
	assert.Equal(t, l0.Id.Token, rm0.Forwards[0].DstAddress)
	assert.Equal(t, l0.Id.Token, rm0.Forwards[1].SrcAddress)
	assert.Equal(t, circuit.IngressId, rm0.Forwards[1].DstAddress)

	// egress route message
	rm1 := routeMessages[1]
	assert.Equal(t, s0.Token, rm1.SessionId)
	assert.NotNil(t, rm1.Egress)
	assert.Equal(t, circuit.EgressId, rm1.Egress.Address)
	assert.Equal(t, addr, rm1.Egress.Destination)
	assert.Equal(t, circuit.EgressId, rm1.Forwards[0].SrcAddress)
	assert.Equal(t, l0.Id.Token, rm1.Forwards[0].DstAddress)
	assert.Equal(t, l0.Id.Token, rm1.Forwards[1].SrcAddress)
	assert.Equal(t, circuit.EgressId, rm1.Forwards[1].DstAddress)
}

func TestTransitCircuit2(t *testing.T) {
	ctx := db.NewTestContext(t)
	defer ctx.Cleanup()

	network, err := NewNetwork(nil, nil, ctx.GetDb(), nil)
	assert.Nil(t, err)

	addr := "tcp:0.0.0.0:0"
	transportAddr, err := tcp.AddressParser{}.Parse(addr)
	assert.Nil(t, err)

	r0 := newRouter("r0", "", transportAddr, nil)
	network.Routers.markConnected(r0)

	r1 := newRouter("r1", "", transportAddr, nil)
	network.Routers.markConnected(r1)

	r2 := newRouter("r2", "", transportAddr, nil)
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

	circuit, err := network.CreateCircuit(r0, r2)
	assert.NotNil(t, circuit)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(circuit.Path))
	assert.Equal(t, r0, circuit.Path[0])
	assert.Equal(t, r1, circuit.Path[1])
	assert.Equal(t, r2, circuit.Path[2])
	assert.Equal(t, 2, len(circuit.Links))
	assert.Equal(t, l0, circuit.Links[0])
	assert.Equal(t, l1, circuit.Links[1])
	assert.Equal(t, r2, circuit.EgressRouter())

	s0 := &identity.TokenId{Token: "s0"}
	routeMessages, err := circuit.CreateRouteMessages(s0, addr)
	assert.NotNil(t, routeMessages)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(routeMessages))

	// ingress route message
	rm0 := routeMessages[0]
	assert.Equal(t, s0.Token, rm0.SessionId)
	assert.Nil(t, rm0.Egress)
	assert.Equal(t, 2, len(rm0.Forwards))
	assert.Equal(t, circuit.IngressId, rm0.Forwards[0].SrcAddress)
	assert.Equal(t, l0.Id.Token, rm0.Forwards[0].DstAddress)
	assert.Equal(t, l0.Id.Token, rm0.Forwards[1].SrcAddress)
	assert.Equal(t, circuit.IngressId, rm0.Forwards[1].DstAddress)

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
	assert.Equal(t, circuit.EgressId, rm2.Egress.Address)
	assert.Equal(t, transportAddr.String(), rm2.Egress.Destination)
	assert.Equal(t, circuit.EgressId, rm2.Forwards[0].SrcAddress)
	assert.Equal(t, l1.Id.Token, rm2.Forwards[0].DstAddress)
	assert.Equal(t, l1.Id.Token, rm2.Forwards[1].SrcAddress)
	assert.Equal(t, circuit.EgressId, rm2.Forwards[1].DstAddress)
}
