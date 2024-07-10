package network

import (
	"fmt"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/xt"
	"github.com/pkg/errors"
	"math"
	"time"
)

func (network *Network) CreateRouteMessages(path *model.Path, attempt uint32, circuitId string, terminator xt.Terminator, deadline time.Time) []*ctrl_pb.Route {
	var routeMessages []*ctrl_pb.Route
	remainingTime := time.Until(deadline)
	if len(path.Links) == 0 {
		// single router path
		routeMessage := &ctrl_pb.Route{CircuitId: circuitId, Attempt: attempt, Timeout: uint64(remainingTime)}
		routeMessage.Forwards = append(routeMessage.Forwards, &ctrl_pb.Route_Forward{
			SrcAddress: path.IngressId,
			DstAddress: path.EgressId,
			DstType:    ctrl_pb.DestType_End,
		})
		routeMessage.Forwards = append(routeMessage.Forwards, &ctrl_pb.Route_Forward{
			SrcAddress: path.EgressId,
			DstAddress: path.IngressId,
			DstType:    ctrl_pb.DestType_Start,
		})
		routeMessage.Egress = &ctrl_pb.Route_Egress{
			Binding:     terminator.GetBinding(),
			Address:     path.EgressId,
			Destination: terminator.GetAddress(),
		}
		routeMessages = append(routeMessages, routeMessage)
	}

	for i, link := range path.Links {
		if i == 0 {
			// ingress
			routeMessage := &ctrl_pb.Route{CircuitId: circuitId, Attempt: attempt, Timeout: uint64(remainingTime)}
			routeMessage.Forwards = append(routeMessage.Forwards, &ctrl_pb.Route_Forward{
				SrcAddress: path.IngressId,
				DstAddress: link.Id,
				DstType:    ctrl_pb.DestType_Link,
			})
			routeMessage.Forwards = append(routeMessage.Forwards, &ctrl_pb.Route_Forward{
				SrcAddress: link.Id,
				DstAddress: path.IngressId,
				DstType:    ctrl_pb.DestType_Start,
			})
			routeMessages = append(routeMessages, routeMessage)
		}
		if i >= 0 && i < len(path.Links)-1 {
			// transit
			nextLink := path.Links[i+1]
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
		if i == len(path.Links)-1 {
			// egress
			routeMessage := &ctrl_pb.Route{CircuitId: circuitId, Attempt: attempt, Timeout: uint64(remainingTime)}
			if attempt != SmartRerouteAttempt {
				routeMessage.Egress = &ctrl_pb.Route_Egress{
					Binding:     terminator.GetBinding(),
					Address:     path.EgressId,
					Destination: terminator.GetAddress(),
				}
			}
			routeMessage.Forwards = append(routeMessage.Forwards, &ctrl_pb.Route_Forward{
				SrcAddress: path.EgressId,
				DstAddress: link.Id,
				DstType:    ctrl_pb.DestType_Link,
			})
			routeMessage.Forwards = append(routeMessage.Forwards, &ctrl_pb.Route_Forward{
				SrcAddress: link.Id,
				DstAddress: path.EgressId,
				DstType:    ctrl_pb.DestType_End,
			})
			routeMessages = append(routeMessages, routeMessage)
		}
	}
	return routeMessages
}

func (network *Network) CreatePathWithNodes(nodes []*model.Router) (*model.Path, CircuitError) {
	ingressId, err := network.sequence.NextHash()
	if err != nil {
		return nil, newCircuitErrWrap(CircuitFailureIdGenerationError, err)
	}

	egressId, err := network.sequence.NextHash()
	if err != nil {
		return nil, newCircuitErrWrap(CircuitFailureIdGenerationError, err)
	}

	path := &model.Path{
		Nodes:     nodes,
		IngressId: ingressId,
		EgressId:  egressId,
	}
	if err := network.setLinks(path); err != nil {
		return nil, newCircuitErrWrap(CircuitFailurePathMissingLink, err)
	}
	return path, nil
}

func (network *Network) UpdatePath(path *model.Path) (*model.Path, error) {
	srcR := path.Nodes[0]
	dstR := path.Nodes[len(path.Nodes)-1]
	nodes, _, err := network.shortestPath(srcR, dstR)
	if err != nil {
		return nil, err
	}

	path2 := &model.Path{
		Nodes:                nodes,
		IngressId:            path.IngressId,
		EgressId:             path.EgressId,
		InitiatorLocalAddr:   path.InitiatorLocalAddr,
		InitiatorRemoteAddr:  path.InitiatorRemoteAddr,
		TerminatorLocalAddr:  path.TerminatorLocalAddr,
		TerminatorRemoteAddr: path.TerminatorRemoteAddr,
	}
	if err := network.setLinks(path2); err != nil {
		return nil, err
	}
	return path2, nil
}

func (network *Network) shortestPath(srcR *model.Router, dstR *model.Router) ([]*model.Router, int64, error) {
	if srcR == nil || dstR == nil {
		return nil, 0, errors.New("not routable (!srcR||!dstR)")
	}

	if srcR == dstR {
		return []*model.Router{srcR}, 0, nil
	}

	dist := make(map[*model.Router]int64)
	prev := make(map[*model.Router]*model.Router)
	unvisited := make(map[*model.Router]bool)

	for _, r := range network.Router.AllConnected() {
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

		neighbors := network.Link.ConnectedNeighborsOfRouter(u)
		for _, r := range neighbors {
			if _, found := unvisited[r]; found {
				var cost int64 = math.MaxInt32 + 1
				if l, found := network.Link.LeastExpensiveLink(r, u); found {
					if !r.NoTraversal || r == srcR || r == dstR {
						cost = l.GetCost() + int64(max(r.Cost, minRouterCost))
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

	routerPath := make([]*model.Router, 0)
	p := prev[dstR]
	for p != nil {
		routerPath = append([]*model.Router{p}, routerPath...)
		p = prev[p]
	}
	routerPath = append(routerPath, dstR)

	if routerPath[0] != srcR {
		return nil, 0, fmt.Errorf("can't route from %v -> %v", srcR.Id, dstR.Id)
	}
	if routerPath[len(routerPath)-1] != dstR {
		return nil, 0, fmt.Errorf("can't route from %v -> %v. destination unreachable", srcR.Id, dstR.Id)
	}

	return routerPath, dist[dstR], nil
}
