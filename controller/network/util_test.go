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
	"fmt"
	"os"
	"sort"

	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/foundation/transport"
	"github.com/openziti/foundation/transport/tcp"
)

func newTestEntityHelper(ctx *db.TestContext, network *Network) *testEntityHelper {
	addr := "tcp:0.0.0.0:0"
	transportAddr, err := tcp.AddressParser{}.Parse(addr)
	ctx.NoError(err)

	return &testEntityHelper{
		network:       network,
		transportAddr: transportAddr,
	}
}

type testEntityHelper struct {
	network       *Network
	routerIdx     int
	linkIdx       int
	transportAddr transport.Address
}

func (self *testEntityHelper) addTestRouter() *Router {
	router := newRouterForTest(fmt.Sprintf("router-%03d", self.routerIdx), "", self.transportAddr, nil, 0)
	self.network.Routers.markConnected(router)
	self.routerIdx++
	return router
}

func (self *testEntityHelper) discardControllerEvents() {
	for {
		select {
		case <-self.network.routerChanged:
		case <-self.network.linkChanged:
		default:
			return
		}
	}
}

// these debug methods can be used to dump routing evaluation steps to a file for easier analysis

func initDebug(path string) {
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	dbg = &debugger{f: f}
}

func stopDebug() {
	if err := dbg.f.Close(); err != nil {
		panic(err)
	}
}

var dbg *debugger

type debugger struct {
	f   *os.File
	err error
}

func debugf(v string, args ...interface{}) {
	if dbg.err == nil {
		_, dbg.err = fmt.Fprintf(dbg.f, v, args...)
	}
}

func debugDumpDistance(dist map[*Router]int64) {
	var keys []*Router
	for k := range dist {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].Id < keys[j].Id
	})
	for _, k := range keys {
		debugf("   ->%v = %v\n", k.Id, dist[k])
	}
}

func debugPath(p *Path) {
	nodes := p.Nodes
	debugf("[r/%v]", nodes[0].Id)
	if len(p.Links) > 0 {
		nodes = nodes[1:]
		for _, link := range p.Links {
			debugf(" -> [l/%v cost=%v] -> [r/%v]", link.Id, link.Cost, nodes[0].Id)
		}
	}
	debugf("\n")
}

func debugNetwork(n *Network) {
	routers := n.AllConnectedRouters()
	sort.Slice(routers, func(i, j int) bool {
		return routers[i].Id < routers[j].Id
	})

	for rIdx, router := range routers {
		debugf("%v router: %v\n", rIdx, router.Id)
		links := router.routerLinks.GetLinks()
		sort.Slice(links, func(i, j int) bool {
			return links[i].Id < links[j].Id
		})
		for lIdx, link := range links {
			debugf("    %v link %v for (%v -> %v) c: %v sc: %v sl:%v dl: %v\n",
				lIdx, link.Id, link.Src.Id, link.Dst.Id, link.GetCost(), link.StaticCost, link.SrcLatency, link.DstLatency)
		}
	}
}
