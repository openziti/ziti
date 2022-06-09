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
	"math"
	"time"

	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/pkg/errors"
)

type Path struct {
	Nodes               []*Router
	Links               []*Link
	IngressId           string
	EgressId            string
	TerminatorLocalAddr string
}

func (self *Path) String() string {
	if len(self.Nodes) < 1 {
		return "{}"
	}
	if len(self.Links) != len(self.Nodes)-1 {
		return "{malformed}"
	}
	out := fmt.Sprintf("[r/%s]", self.Nodes[0].Id)
	for i := 0; i < len(self.Links); i++ {
		out += fmt.Sprintf("->[l/%s]", self.Links[i].Id)
		out += fmt.Sprintf("->[r/%s]", self.Nodes[i+1].Id)
	}
	return out
}

func (self *Path) EqualPath(other *Path) bool {
	if len(self.Nodes) != len(other.Nodes) {
		return false
	}
	if len(self.Links) != len(other.Links) {
		return false
	}
	for i := 0; i < len(self.Nodes); i++ {
		if self.Nodes[i] != other.Nodes[i] {
			return false
		}
	}
	for i := 0; i < len(self.Links); i++ {
		if self.Links[i] != other.Links[i] {
			return false
		}
	}
	return true
}

func (self *Path) EgressRouter() *Router {
	if len(self.Nodes) > 0 {
		return self.Nodes[len(self.Nodes)-1]
	}
	return nil
}

func (self *Path) CreateRouteMessages(attempt uint32, circuitId string, terminator xt.Terminator, deadline time.Time) []*ctrl_pb.Route {
	var routeMessages []*ctrl_pb.Route
	remainingTime := deadline.Sub(time.Now())
	if len(self.Links) == 0 {
		// single router path
		routeMessage := &ctrl_pb.Route{CircuitId: circuitId, Attempt: attempt, Timeout: uint64(remainingTime)}
		routeMessage.Forwards = append(routeMessage.Forwards, &ctrl_pb.Route_Forward{
			SrcAddress: self.IngressId,
			DstAddress: self.EgressId,
			DstType:    ctrl_pb.DestType_End,
		})
		routeMessage.Forwards = append(routeMessage.Forwards, &ctrl_pb.Route_Forward{
			SrcAddress: self.EgressId,
			DstAddress: self.IngressId,
			DstType:    ctrl_pb.DestType_Start,
		})
		routeMessage.Egress = &ctrl_pb.Route_Egress{
			Binding:     terminator.GetBinding(),
			Address:     self.EgressId,
			Destination: terminator.GetAddress(),
		}
		routeMessages = append(routeMessages, routeMessage)
	}

	for i, link := range self.Links {
		if i == 0 {
			// ingress
			routeMessage := &ctrl_pb.Route{CircuitId: circuitId, Attempt: attempt, Timeout: uint64(remainingTime)}
			routeMessage.Forwards = append(routeMessage.Forwards, &ctrl_pb.Route_Forward{
				SrcAddress: self.IngressId,
				DstAddress: link.Id,
				DstType:    ctrl_pb.DestType_Link,
			})
			routeMessage.Forwards = append(routeMessage.Forwards, &ctrl_pb.Route_Forward{
				SrcAddress: link.Id,
				DstAddress: self.IngressId,
				DstType:    ctrl_pb.DestType_Start,
			})
			routeMessages = append(routeMessages, routeMessage)
		}
		if i >= 0 && i < len(self.Links)-1 {
			// transit
			nextLink := self.Links[i+1]
			routeMessage := &ctrl_pb.Route{CircuitId: circuitId, Attempt: attempt, Timeout: uint64(remainingTime)}
			routeMessage.Forwards = append(routeMessage.Forwards, &ctrl_pb.Route_Forward{
				SrcAddress: link.Id,
				DstAddress: nextLink.Id,
				DstType:    ctrl_pb.DestType_Link,
			})
			routeMessage.Forwards = append(routeMessage.Forwards, &ctrl_pb.Route_Forward{
				SrcAddress: nextLink.Id,
				DstAddress: link.Id,
				DstType:    ctrl_pb.DestType_Link,
			})
			routeMessages = append(routeMessages, routeMessage)
		}
		if i == len(self.Links)-1 {
			// egress
			routeMessage := &ctrl_pb.Route{CircuitId: circuitId, Attempt: attempt, Timeout: uint64(remainingTime)}
			if attempt != SmartRerouteAttempt {
				routeMessage.Egress = &ctrl_pb.Route_Egress{
					Binding:     terminator.GetBinding(),
					Address:     self.EgressId,
					Destination: terminator.GetAddress(),
				}
			}
			routeMessage.Forwards = append(routeMessage.Forwards, &ctrl_pb.Route_Forward{
				SrcAddress: self.EgressId,
				DstAddress: link.Id,
				DstType:    ctrl_pb.DestType_Link,
			})
			routeMessage.Forwards = append(routeMessage.Forwards, &ctrl_pb.Route_Forward{
				SrcAddress: link.Id,
				DstAddress: self.EgressId,
				DstType:    ctrl_pb.DestType_End,
			})
			routeMessages = append(routeMessages, routeMessage)
		}
	}
	return routeMessages
}

func (self *Path) usesLink(l *Link) bool {
	if self.Links != nil {
		for _, o := range self.Links {
			if o == l {
				return true
			}
		}
	}
	return false
}

func (network *Network) shortestPath(srcR *Router, dstR *Router) ([]*Router, int64, error) {
	if srcR == nil || dstR == nil {
		return nil, 0, errors.New("not routable (!srcR||!dstR)")
	}

	if srcR == dstR {
		return []*Router{srcR}, 0, nil
	}

	dist := make(map[*Router]int64)
	prev := make(map[*Router]*Router)
	unvisited := make(map[*Router]bool)

	for _, r := range network.Routers.allConnected() {
		dist[r] = math.MaxInt32
		unvisited[r] = true
	}
	dist[srcR] = 0

	minRouterCost := network.options.MinRouterCost

	for len(unvisited) > 0 {
		u := minCost(unvisited, dist)
		if u == dstR { // if the dest router is the lowest cost next link, we can stop evaluating
			break
		}
		delete(unvisited, u)

		neighbors := network.linkController.connectedNeighborsOfRouter(u)
		for _, r := range neighbors {
			if _, found := unvisited[r]; found {
				var cost int64 = math.MaxInt32 + 1
				if l, found := network.linkController.leastExpensiveLink(r, u); found {
					if !r.NoTraversal || r == srcR || r == dstR {
						cost = l.GetCost() + int64(maxUint16(r.Cost, minRouterCost))
					}
				}

				alt := dist[u] + cost
				if alt < dist[r] {
					dist[r] = alt
					prev[r] = u
				}
			}
		}
	}

	/*
	 * dist: (r2->r1->r0)
	 *		r0 = 2 <- r1
	 *		r1 = 1 <- r2
	 *		r2 = 0 <- nil
	 */

	routerPath := make([]*Router, 0)
	p := prev[dstR]
	for p != nil {
		routerPath = append([]*Router{p}, routerPath...)
		p = prev[p]
	}
	routerPath = append(routerPath, dstR)

	if routerPath[0] != srcR {
		return nil, 0, fmt.Errorf("can't route from %v -> %v. source unreachable", srcR.Id, dstR.Id)
	}
	if routerPath[len(routerPath)-1] != dstR {
		return nil, 0, fmt.Errorf("can't route from %v -> %v. destination unreachable", srcR.Id, dstR.Id)
	}

	return routerPath, dist[dstR], nil
}

func minCost(q map[*Router]bool, dist map[*Router]int64) *Router {
	if dist == nil || len(dist) < 1 {
		return nil
	}

	min := int64(math.MaxInt64)
	var selected *Router
	for r := range q {
		d := dist[r]
		if d <= min {
			selected = r
			min = d
		}
	}
	return selected
}

func maxUint16(v1, v2 uint16) uint16 {
	if v1 > v2 {
		return v1
	}
	return v2
}
