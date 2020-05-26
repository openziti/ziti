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

package handler_ctrl

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/router/forwarder"
	"github.com/openziti/fabric/router/handler_xgress"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/identity/identity"
)

type routeHandler struct {
	id        *identity.TokenId
	ctrl      xgress.CtrlChannel
	dialerCfg map[string]xgress.OptionsData
	forwarder *forwarder.Forwarder
}

func newRouteHandler(id *identity.TokenId, ctrl xgress.CtrlChannel, dialerCfg map[string]xgress.OptionsData, forwarder *forwarder.Forwarder) *routeHandler {
	return &routeHandler{
		id:        id,
		ctrl:      ctrl,
		dialerCfg: dialerCfg,
		forwarder: forwarder,
	}
}

func (rh *routeHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_RouteType)
}

func (rh *routeHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	log := pfxlog.ContextLogger(ch.Label())

	route := &ctrl_pb.Route{}
	if err := proto.Unmarshal(msg.Body, route); err == nil {
		if err := rh.createRoute(route); err == nil {
			response := channel2.NewResult(true, "")
			response.ReplyTo(msg)
			if err := rh.ctrl.Channel().Send(response); err == nil {
				log.Debugf("handled route for [s/%s]", route.SessionId)
			} else {
				log.Errorf("send response failed for [s/%s] (%s)", route.SessionId, err)
			}
		} else {
			pfxlog.ContextLogger(ch.Label()).Errorf("error creating route for [s/%s] (%s)", route.SessionId, err)
		}

	} else {
		pfxlog.ContextLogger(ch.Label()).Errorf("error unmarshaling (%s)", err)
	}
}

func (rh *routeHandler) createRoute(route *ctrl_pb.Route) error {
	if route.Egress != nil {
		if rh.forwarder.HasDestination(xgress.Address(route.Egress.Address)) {
			pfxlog.Logger().Warnf("destination exists for [%s]", route.Egress.Address)
		} else {
			if err := rh.connectEgress(route); err != nil {
				return err
			}
		}
	}

	rh.forwarder.Route(route)

	return nil
}

func (rh *routeHandler) connectEgress(route *ctrl_pb.Route) error {
	log := pfxlog.Logger().WithField("sessionId", route.SessionId)
	log.Debugf("route request received. binding: %v, destination: %v, address: %v",
		route.Egress.Binding, route.Egress.Destination, route.Egress.Address)
	if factory, err := xgress.GlobalRegistry().Factory(route.Egress.Binding); err == nil {
		if dialer, err := factory.CreateDialer(rh.dialerCfg[route.Egress.Binding]); err == nil {
			sessionId := &identity.TokenId{Token: route.SessionId, Data: route.Egress.PeerData}
			return dialer.Dial(route.Egress.Destination,
				sessionId,
				xgress.Address(route.Egress.Address),
				handler_xgress.NewBindHandler(
					handler_xgress.NewReceiveHandler(rh.ctrl, rh.forwarder),
					handler_xgress.NewCloseHandler(rh.ctrl, rh.forwarder),
					rh.forwarder))
		} else {
			return fmt.Errorf("unable to create dialer (%s)", err)
		}

	} else {
		return err
	}
}
