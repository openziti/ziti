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
	"github.com/golang/protobuf/proto"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/ctrl_msg"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/foundation/channel2"
	"github.com/sirupsen/logrus"
)

type routeResultHandler struct {
	network *network.Network
	r       *network.Router
}

func newRouteResultHandler(network *network.Network, r *network.Router) *routeResultHandler {
	return &routeResultHandler{
		network: network,
		r:       r,
	}
}

func (self *routeResultHandler) ContentType() int32 {
	return ctrl_msg.RouteResultType
}

func (self *routeResultHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	_, success := msg.Headers[ctrl_msg.RouteResultSuccessHeader]
	sessionId := string(msg.Body)
	routing := self.network.RouteResult(self.r, sessionId, success)
	if !routing {
		go self.notRoutingSession(sessionId)
	}
}

func (self *routeResultHandler) notRoutingSession(sessionId string) {
	logrus.Warnf("not routing session [s/%s] for router [r/%s], sending unroute", sessionId, self.r.Id)
	unroute := &ctrl_pb.Unroute{
		SessionId: sessionId,
		Now:       true,
	}
	if body, err := proto.Marshal(unroute); err == nil {
		unrouteMsg := channel2.NewMessage(int32(ctrl_pb.ContentType_UnrouteType), body)
		if err := self.r.Control.Send(unrouteMsg); err != nil {
			logrus.Errorf("error sending unroute message for [s/%s] to [r/%s] (%v)", sessionId, self.r.Id, err)
		}
	} else {
		logrus.Errorf("error sending unroute message for [s/%s] to [r/%s] (%v)", sessionId, self.r.Id, err)
	}
}
