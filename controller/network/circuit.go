/*
	Copyright 2019 Netfoundry, Inc.

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
	"github.com/netfoundry/ziti-fabric/pb/ctrl_pb"
	"github.com/netfoundry/ziti-foundation/identity/identity"
)

type Circuit struct {
	Path      []*Router
	Links     []*Link
	Binding   string
	IngressId string
	EgressId  string
}

func (circuit *Circuit) String() string {
	if len(circuit.Path) < 1 {
		return "{}"
	}
	if len(circuit.Links) != len(circuit.Path) - 1 {
		return "{malformed}"
	}
	out := fmt.Sprintf("[r/%s]", circuit.Path[0].Id)
	for i := 0; i < len(circuit.Links); i++ {
		out += fmt.Sprintf("->[l/%s]", circuit.Links[i].Id.Token)
		out += fmt.Sprintf("->[r/%s]", circuit.Path[i+1].Id)
	}
	return out
}

func (circuit *Circuit) EqualPath(other *Circuit) bool {
	if len(circuit.Path) != len(other.Path) {
		return false
	}
	if len(circuit.Links) != len(other.Links) {
		return false
	}
	for i := 0; i < len(circuit.Path); i++ {
		if circuit.Path[i] != other.Path[i] {
			return false
		}
	}
	for i := 0; i < len(circuit.Links); i++ {
		if circuit.Links[i] != other.Links[i] {
			return false
		}
	}
	return true
}

func (circuit *Circuit) EgressRouter() *Router {
	if len(circuit.Path) > 0 {
		return circuit.Path[len(circuit.Path)-1]
	}
	return nil
}

func (circuit *Circuit) CreateRouteMessages(sessionId *identity.TokenId, egressAddress string) ([]*ctrl_pb.Route, error) {
	var routeMessages []*ctrl_pb.Route
	if len(circuit.Links) == 0 {
		// single router path
		routeMessage := &ctrl_pb.Route{SessionId: sessionId.Token}
		routeMessage.Forwards = append(routeMessage.Forwards, &ctrl_pb.Route_Forward{
			SrcAddress: circuit.IngressId,
			DstAddress: circuit.EgressId,
		})
		routeMessage.Forwards = append(routeMessage.Forwards, &ctrl_pb.Route_Forward{
			SrcAddress: circuit.EgressId,
			DstAddress: circuit.IngressId,
		})
		routeMessage.Egress = &ctrl_pb.Route_Egress{
			Binding:     circuit.Binding,
			Address:     circuit.EgressId,
			Destination: egressAddress,
		}
		routeMessages = append(routeMessages, routeMessage)
	}

	for i, link := range circuit.Links {
		if i == 0 {
			// ingress
			routeMessage := &ctrl_pb.Route{SessionId: sessionId.Token}
			routeMessage.Forwards = append(routeMessage.Forwards, &ctrl_pb.Route_Forward{
				SrcAddress: circuit.IngressId,
				DstAddress: link.Id.Token,
			})
			routeMessage.Forwards = append(routeMessage.Forwards, &ctrl_pb.Route_Forward{
				SrcAddress: link.Id.Token,
				DstAddress: circuit.IngressId,
			})
			routeMessages = append(routeMessages, routeMessage)
		}
		if i >= 0 && i < len(circuit.Links)-1 {
			// transit
			nextLink := circuit.Links[i+1]
			routeMessage := &ctrl_pb.Route{SessionId: sessionId.Token}
			routeMessage.Forwards = append(routeMessage.Forwards, &ctrl_pb.Route_Forward{
				SrcAddress: link.Id.Token,
				DstAddress: nextLink.Id.Token,
			})
			routeMessage.Forwards = append(routeMessage.Forwards, &ctrl_pb.Route_Forward{
				SrcAddress: nextLink.Id.Token,
				DstAddress: link.Id.Token,
			})
			routeMessages = append(routeMessages, routeMessage)
		}
		if i == len(circuit.Links)-1 {
			// egress
			routeMessage := &ctrl_pb.Route{SessionId: sessionId.Token}
			routeMessage.Egress = &ctrl_pb.Route_Egress{
				Binding:     circuit.Binding,
				Address:     circuit.EgressId,
				Destination: egressAddress,
			}
			routeMessage.Forwards = append(routeMessage.Forwards, &ctrl_pb.Route_Forward{
				SrcAddress: circuit.EgressId,
				DstAddress: link.Id.Token,
			})
			routeMessage.Forwards = append(routeMessage.Forwards, &ctrl_pb.Route_Forward{
				SrcAddress: link.Id.Token,
				DstAddress: circuit.EgressId,
			})
			routeMessages = append(routeMessages, routeMessage)
		}
	}
	return routeMessages, nil
}

func (circuit *Circuit) usesLink(l *Link) bool {
	if circuit.Links != nil {
		for _, o := range circuit.Links {
			if o == l {
				return true
			}
		}
	}
	return false
}
