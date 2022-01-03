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
	"bytes"
	"encoding/binary"
	"github.com/golang/protobuf/proto"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/controller/xt"
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

func (self *routeResultHandler) HandleReceive(msg *channel2.Message, _ channel2.Channel) {
	go self.handleRouteResult(msg)
}

func (self *routeResultHandler) handleRouteResult(msg *channel2.Message) {
	log := logrus.WithField("routerId", self.r.Id)
	if v, found := msg.Headers[ctrl_msg.RouteResultAttemptHeader]; found {
		_, success := msg.Headers[ctrl_msg.RouteResultSuccessHeader]
		rerrv, _ := msg.Headers[ctrl_msg.RouteResultErrorHeader]
		rerr := string(rerrv)

		var attempt uint32
		buf := bytes.NewBuffer(v)
		if err := binary.Read(buf, binary.LittleEndian, &attempt); err == nil {
			circuitId := string(msg.Body)
			peerData := xt.PeerData{}
			for k, v := range msg.Headers {
				if k > 0 && k != ctrl_msg.RouteResultSuccessHeader && k != ctrl_msg.RouteResultErrorHeader && k != ctrl_msg.RouteResultAttemptHeader {
					peerData[uint32(k)] = v
				}
			}
			routing := self.network.RouteResult(self.r, circuitId, attempt, success, rerr, peerData)
			if !routing && attempt != network.SmartRerouteAttempt {
				go self.notRoutingCircuit(circuitId)
			}

		} else {
			log.WithError(err).Error("error reading attempt number from route result")
			return
		}
	} else {
		log.Error("missing attempt header in route result")
	}
}

func (self *routeResultHandler) notRoutingCircuit(circuitId string) {
	log := logrus.WithField("circuitId", circuitId).
		WithField("routerId", self.r.Id)
	log.Warn("not routing circuit (and not smart re-route), sending unroute")
	unroute := &ctrl_pb.Unroute{
		CircuitId: circuitId,
		Now:       true,
	}
	if body, err := proto.Marshal(unroute); err == nil {
		unrouteMsg := channel2.NewMessage(int32(ctrl_pb.ContentType_UnrouteType), body)
		if err := self.r.Control.Send(unrouteMsg); err != nil {
			log.WithError(err).Error("error sending unroute message to router")
		}
	} else {
		log.WithError(err).Error("error sending unroute message to router")
	}
}
