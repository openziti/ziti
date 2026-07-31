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

	"github.com/openziti/transport/v2/tcp"
	"github.com/stretchr/testify/assert"
)

// TestBuildTakeoverPathMultiRouter covers the ordinary case: a circuit r0->r1 whose dialer moves to
// a new ingress r2 (with a link to the terminator router r1). The new path must run r2->r1, use the
// router-allocated ingress id, and preserve the terminator side (EgressId and terminator addrs).
func TestBuildTakeoverPathMultiRouter(t *testing.T) {
	ctx := model.NewTestContext(t)
	defer ctx.Cleanup()

	config := newTestConfig(ctx)
	defer close(config.closeNotify)

	network, err := NewNetwork(config, ctx)
	assert.Nil(t, err)

	transportAddr, err := tcp.AddressParser{}.Parse("tcp:0.0.0.0:0")
	assert.Nil(t, err)

	r0 := model.NewRouterForTest("r0", "", transportAddr, nil, 0, false) // old ingress
	network.Router.MarkConnected(r0)
	r1 := model.NewRouterForTest("r1", "", transportAddr, nil, 0, false) // terminator
	network.Router.MarkConnected(r1)
	r2 := model.NewRouterForTest("r2", "", transportAddr, nil, 0, false) // new ingress
	network.Router.MarkConnected(r2)

	// old path ingress link r0-r1, and the new-ingress link r2-r1
	l0 := model.NewTestLink("l0", r0, r1)
	l0.SetState(model.Connected)
	network.Link.Add(l0)
	l1 := model.NewTestLink("l1", r2, r1)
	l1.SetState(model.Connected)
	network.Link.Add(l1)

	oldPath := &model.Path{
		Nodes:                []*model.Router{r0, r1},
		Links:                []*model.Link{l0},
		IngressId:            "ingress-old",
		EgressId:             "egress-preserved",
		TerminatorLocalAddr:  "term-local",
		TerminatorRemoteAddr: "term-remote",
		InitiatorLocalAddr:   "init-local-old",
		InitiatorRemoteAddr:  "init-remote-old",
	}

	newPath, err := network.BuildTakeoverPath(oldPath, r2)
	assert.NoError(t, err)
	assert.NotNil(t, newPath)

	assert.Equal(t, r2, newPath.Nodes[0], "new ingress must be r2")
	assert.Equal(t, r1, newPath.Nodes[len(newPath.Nodes)-1], "terminator router must be unchanged")
	assert.Equal(t, "ingress-old", newPath.IngressId, "ingress id must be preserved from the old path")
	assert.Equal(t, "egress-preserved", newPath.EgressId, "egress id must be preserved from the old path")
	assert.Equal(t, "term-local", newPath.TerminatorLocalAddr)
	assert.Equal(t, "term-remote", newPath.TerminatorRemoteAddr)
	assert.Equal(t, 1, len(newPath.Links))
	assert.Equal(t, l1, newPath.Links[0], "new path must use the r2-r1 link")
}

// TestBuildTakeoverPathSingleRouterToTwoRouter covers a single-router circuit (ingress and
// terminator both on r0) whose dialer comes back via a different ingress r2 with a link to r0. The
// takeover transforms it into a two-router circuit r2->r0 while preserving the terminator's egress.
func TestBuildTakeoverPathSingleRouterToTwoRouter(t *testing.T) {
	ctx := model.NewTestContext(t)
	defer ctx.Cleanup()

	config := newTestConfig(ctx)
	defer close(config.closeNotify)

	network, err := NewNetwork(config, ctx)
	assert.Nil(t, err)

	transportAddr, err := tcp.AddressParser{}.Parse("tcp:0.0.0.0:0")
	assert.Nil(t, err)

	r0 := model.NewRouterForTest("r0", "", transportAddr, nil, 0, false) // single-router ingress+terminator
	network.Router.MarkConnected(r0)
	r2 := model.NewRouterForTest("r2", "", transportAddr, nil, 0, false) // new ingress
	network.Router.MarkConnected(r2)

	l0 := model.NewTestLink("l0", r2, r0)
	l0.SetState(model.Connected)
	network.Link.Add(l0)

	oldPath := &model.Path{
		Nodes:                []*model.Router{r0},
		IngressId:            "ingress-old",
		EgressId:             "egress-preserved",
		TerminatorLocalAddr:  "term-local",
		TerminatorRemoteAddr: "term-remote",
	}

	newPath, err := network.BuildTakeoverPath(oldPath, r2)
	assert.NoError(t, err)
	assert.NotNil(t, newPath)

	assert.Equal(t, r2, newPath.Nodes[0], "new ingress must be r2")
	assert.Equal(t, r0, newPath.Nodes[len(newPath.Nodes)-1], "terminator router r0 must be unchanged")
	assert.Equal(t, "ingress-old", newPath.IngressId, "ingress id must be preserved")
	assert.Equal(t, "egress-preserved", newPath.EgressId, "egress id must be preserved so terminator forwarding survives")
	assert.Equal(t, 1, len(newPath.Links))
}
