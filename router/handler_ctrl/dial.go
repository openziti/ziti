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
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/router/forwarder"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/fabric/router/xlink"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/identity/identity"
	"github.com/sirupsen/logrus"
)

type dialHandler struct {
	id      *identity.TokenId
	ctrl    xgress.CtrlChannel
	dialers []xlink.Dialer
	pool    handlerPool
}

func newDialHandler(id *identity.TokenId, ctrl xgress.CtrlChannel, dialers []xlink.Dialer, forwarder *forwarder.Forwarder) *dialHandler {
	handler := &dialHandler{
		id:      id,
		ctrl:    ctrl,
		dialers: dialers,
		pool:    handlerPool{options: forwarder.Options.LinkDial},
	}
	handler.pool.Start()

	return handler
}

func (self *dialHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_DialType)
}

func (self *dialHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	self.pool.Queue(func(){
		self.handle(msg, ch)
	})
}

func (self *dialHandler) handle(msg *channel2.Message, _ channel2.Channel) {
	logrus.Info("received link connect request")
	dial := &ctrl_pb.Dial{}
	if err := proto.Unmarshal(msg.Body, dial); err == nil {
		linkId := self.id.ShallowCloneWithNewToken(dial.Id)
		if len(self.dialers) == 1 {
			if err := self.dialers[0].Dial(dial.Address, linkId); err == nil {
				if err := self.sendLinkMessage(linkId); err != nil {
					logrus.Errorf("error sending link message [l/%s] (%v)", linkId.Token, err)
				}
				logrus.Infof("link [l/%s] established", linkId.Token)

			} else {
				logrus.Errorf("link dialing failed [%s] (%v)", dial.Address, err)
				if err := self.sendLinkFault(linkId); err != nil {
					logrus.Errorf("error sending fault [l/%s] (%v)", linkId.Token, err)
				}
			}
		} else {
			logrus.Errorf("invalid Xlink dialers configuration")
			if err := self.sendLinkFault(linkId); err != nil {
				logrus.Errorf("error sending link fault [l/%s] (%v)", linkId.Token, err)
			}
		}
	} else {
		logrus.Errorf("error unmarshaling dial message (%v)", err)
	}
}

func (self *dialHandler) sendLinkMessage(linkId *identity.TokenId) error {
	linkMsg := &ctrl_pb.Link{Id: linkId.Token}
	body, err := proto.Marshal(linkMsg)
	if err == nil {
		msg := channel2.NewMessage(int32(ctrl_pb.ContentType_LinkType), body)
		if err := self.ctrl.Channel().Send(msg); err != nil {
			return fmt.Errorf("error sending link message (%w)", err)
		}
	} else {
		return fmt.Errorf("error marshaling link message (%w)", err)
	}
	return nil
}

func (self *dialHandler) sendLinkFault(linkId *identity.TokenId) error {
	fault := &ctrl_pb.Fault{Subject: ctrl_pb.FaultSubject_LinkFault, Id: linkId.Token}
	body, err := proto.Marshal(fault)
	if err == nil {
		msg := channel2.NewMessage(int32(ctrl_pb.ContentType_FaultType), body)
		if err := self.ctrl.Channel().Send(msg); err != nil {
			return fmt.Errorf("error sending fault (%w)", err)
		}
	} else {
		return fmt.Errorf("error marshaling fault (%w)", err)
	}
	return nil
}
