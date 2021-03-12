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
	"github.com/openziti/foundation/identity/identity"
	"github.com/sirupsen/logrus"
)

type sessionConfirmationHandler struct {
	n *network.Network
	r       *network.Router
}

func newSessionConfirmationHandler(n *network.Network, r *network.Router) *sessionConfirmationHandler {
	return &sessionConfirmationHandler{n, r}
}

func (self *sessionConfirmationHandler) ContentType() int32 {
	return int32(ctrl_msg.SessionConfirmationType)
}

func (self *sessionConfirmationHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	logrus.Infof("received session confirmation request from [r/%s]", self.r.Id)
	confirm := &ctrl_pb.SessionConfirmation{}
	if err := proto.Unmarshal(msg.Body, confirm); err == nil {
		for _, sessionId := range confirm.SessionIds {
			if _, found := self.n.GetSession(&identity.TokenId{Token: sessionId}); !found {
				go self.sendUnroute(sessionId)
			} else {
				logrus.Debugf("[s/%s] found, ignoring", sessionId)
			}
		}
	} else {
		logrus.Errorf("error unmarshaling session confirmation from [r/%s] (%v)", self.r.Id, err)
	}
}

func (self *sessionConfirmationHandler) sendUnroute(sessionId string) {
	unroute := &ctrl_pb.Unroute{}
	unroute.SessionId = sessionId
	unroute.Now = true
	if body, err := proto.Marshal(unroute); err == nil {
		msg := channel2.NewMessage(int32(ctrl_pb.ContentType_UnrouteType), body)
		if err := self.r.Control.Send(msg); err == nil {
			logrus.Infof("sent unroute to [r/%s] for [s/%s]", self.r.Id, sessionId)
		} else {
			logrus.Errorf("error sending unroute to [r/%s] for [s/%s] (%v)", self.r.Id, sessionId, err)
		}
	} else {
		logrus.Errorf("error marshalling unroute to [r/%s] for [s/%s] (%v)", self.r.Id, sessionId, err)
	}
}