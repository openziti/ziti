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
	"github.com/openziti/foundation/channel"
	"github.com/sirupsen/logrus"
)

type circuitConfirmationHandler struct {
	n *network.Network
	r *network.Router
}

func newCircuitConfirmationHandler(n *network.Network, r *network.Router) *circuitConfirmationHandler {
	return &circuitConfirmationHandler{n, r}
}

func (self *circuitConfirmationHandler) ContentType() int32 {
	return int32(ctrl_msg.CircuitConfirmationType)
}

func (self *circuitConfirmationHandler) HandleReceive(msg *channel.Message, _ channel.Channel) {
	logrus.Infof("received circuit confirmation request from [r/%s]", self.r.Id)
	confirm := &ctrl_pb.CircuitConfirmation{}
	if err := proto.Unmarshal(msg.Body, confirm); err == nil {
		for _, circuitId := range confirm.CircuitIds {
			if _, found := self.n.GetCircuit(circuitId); !found {
				go self.sendUnroute(circuitId)
			} else {
				logrus.Debugf("[s/%s] found, ignoring", circuitId)
			}
		}
	} else {
		logrus.Errorf("error unmarshaling circuit confirmation from [r/%s] (%v)", self.r.Id, err)
	}
}

func (self *circuitConfirmationHandler) sendUnroute(circuitId string) {
	unroute := &ctrl_pb.Unroute{}
	unroute.CircuitId = circuitId
	unroute.Now = true
	if body, err := proto.Marshal(unroute); err == nil {
		msg := channel.NewMessage(int32(ctrl_pb.ContentType_UnrouteType), body)
		if err := self.r.Control.Send(msg); err == nil {
			logrus.Infof("sent unroute to [r/%s] for [s/%s]", self.r.Id, circuitId)
		} else {
			logrus.Errorf("error sending unroute to [r/%s] for [s/%s] (%v)", self.r.Id, circuitId, err)
		}
	} else {
		logrus.Errorf("error marshalling unroute to [r/%s] for [s/%s] (%v)", self.r.Id, circuitId, err)
	}
}
