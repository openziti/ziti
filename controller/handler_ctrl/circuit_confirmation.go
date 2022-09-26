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

package handler_ctrl

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

type circuitConfirmationHandler struct {
	n *network.Network
	r *network.Router
}

func newCircuitConfirmationHandler(n *network.Network, r *network.Router) *circuitConfirmationHandler {
	return &circuitConfirmationHandler{n, r}
}

func (self *circuitConfirmationHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_CircuitConfirmationType)
}

func (self *circuitConfirmationHandler) HandleReceive(msg *channel.Message, _ channel.Channel) {
	log := logrus.WithField("routerId", self.r.Id)
	confirm := &ctrl_pb.CircuitConfirmation{}
	if err := proto.Unmarshal(msg.Body, confirm); err == nil {
		log.WithField("circuitCount", len(confirm.CircuitIds)).Info("received circuit confirmation request")
		for _, circuitId := range confirm.CircuitIds {
			if circuit, found := self.n.GetCircuit(circuitId); found && circuit.HasRouter(self.r.Id) {
				log.WithField("circuitId", circuitId).Debug("circuit found, ignoring")
			} else {
				go self.sendUnroute(circuitId)
			}
		}
	} else {
		log.WithError(err).Error("error unmarshalling circuit confirmation")
	}
}

func (self *circuitConfirmationHandler) sendUnroute(circuitId string) {
	log := pfxlog.Logger().WithField("circuitId", circuitId).WithField("routerId", self.r.Id)
	unroute := &ctrl_pb.Unroute{}
	unroute.CircuitId = circuitId
	unroute.Now = true
	if body, err := proto.Marshal(unroute); err == nil {
		msg := channel.NewMessage(int32(ctrl_pb.ContentType_UnrouteType), body)
		if err = self.r.Control.Send(msg); err == nil {
			log.Info("sent unroute to router for circuit")
		} else {
			log.WithError(err).Error("error sending unroute to router for circuit")
		}
	} else {
		log.WithError(err).Error("error marshalling unroute to router for circuit")
	}
}
